"""
LangChain Agent 模块的 FastAPI 路由

提供独立的 API 端点，区别于现有的 app/api/parse.py（后者使用原始 DeepSeek SDK）。
通过这些端点，Go 后端可以调用 LangChain Agent 流水线。

端点列表：
- POST /api/langchain/diagnose : 完整的诊断流水线（NER -> FactCheck -> Supplement）
- POST /api/langchain/chat     : AI 对话（基于图谱上下文）
- GET  /api/langchain/health   : LangChain 模块健康检查
"""

import json
import logging

from fastapi import APIRouter, HTTPException

from app.langchain_agent.chains import get_diagnosis_chain, DiagnosisResult
from app.langchain_agent.agents.react_chat_agent import get_react_chat_agent
from app.langchain_agent.prompts import get_chat_prompt, get_learning_path_prompt
from app.langchain_agent.tools import format_graph_context, format_dependency_tree
from app.langchain_agent.schemas import DiagnosisOutput
from app.services.markdown_parser import split_markdown_to_chunks
from app.core.config import settings

router = APIRouter(prefix="/api/langchain", tags=["langchain"])
logger = logging.getLogger(__name__)


# ==================== 辅助函数 ====================

def _build_chat_llm(temperature: float = 0.7):
    """构建对话用 LLM 实例（延迟导入 langchain_openai）"""
    from langchain_openai import ChatOpenAI  # noqa: PLC0415
    return ChatOpenAI(
        model=settings.deepseek_model,
        api_key=settings.deepseek_api_key,
        base_url=settings.deepseek_base_url,
        temperature=temperature,
        max_tokens=2048,
        timeout=120,
        max_retries=2,
    )


# ==================== Pydantic 请求/响应模型 ====================

from pydantic import BaseModel, Field


class DiagnoseRequest(BaseModel):
    """诊断流水线请求"""
    markdown: str = Field(..., min_length=1, description="待解析的 Markdown 笔记文本")


class DiagnoseResponse(BaseModel):
    """诊断流水线响应"""
    success: bool
    nodes: list[dict] = Field(default_factory=list)
    edges: list[dict] = Field(default_factory=list)
    summary: str = ""
    retries_used: int = 0
    error_message: str = ""


class ChatRequest(BaseModel):
    """AI 对话请求"""
    user_message: str = Field(..., min_length=1, description="用户输入的消息")
    conversation_id: str = Field(default="", description="对话会话 ID")
    markdown: str = Field(default="", description="当前 Markdown 笔记全文")
    graph_nodes: str = Field(default="[]", description="图谱节点 JSON")
    graph_edges: str = Field(default="[]", description="图谱边 JSON")
    image_base64: str = Field(default="", description="图片 Base64 编码（可选，支持多模态）")


class ChatResponse(BaseModel):
    """AI 对话响应"""
    reply: str = Field(..., description="Agent 的回复内容")
    conversation_id: str = Field(default="", description="对话会话 ID")
    edited_markdown: str = Field(default="", description="Agent 修改后的 Markdown（若有修改）")


class LearningPathRequest(BaseModel):
    """学习路径请求"""
    target_concept: str = Field(..., min_length=1, description="目标高阶概念")
    dependency_tree_json: str = Field(
        default="[]",
        description="Go 后端 DFS 查询返回的依赖树 JSON"
    )
    graph_nodes_json: str = Field(
        default="[]",
        description="图谱中所有相关节点的状态信息 JSON"
    )


class LearningPathResponse(BaseModel):
    """学习路径响应"""
    guidance: str = Field(..., description="Agent 的学习路径指导文本")


# ==================== 端点实现 ====================

@router.post("/diagnose", response_model=DiagnoseResponse)
def diagnose(payload: DiagnoseRequest) -> DiagnoseResponse:
    """
    完整的 LangChain 诊断流水线

    流程：AST切分 -> NER抽取 -> 事实校验 -> 知识补全 -> 聚合输出

    返回符合 Go 后端入库格式的 nodes[] + edges[] 数组，
    包含 status（correct/error/supplement）和 reason 字段。
    """
    # Step 1: AST Semantic Chunking
    chunks = split_markdown_to_chunks(payload.markdown)
    if not any(c.strip() for c in chunks):
        raise HTTPException(status_code=400, detail="Markdown 内容为空")

    # Step 2: 运行 LangChain 诊断流水线
    chain = get_diagnosis_chain()
    try:
        result: DiagnosisResult = chain.run(
            chunks=chunks,
            original_markdown=payload.markdown,
        )
    except Exception as exc:
        logger.exception("[LangChain Router] 诊断流水线执行异常")
        raise HTTPException(status_code=500, detail=f"诊断流水线执行失败: {exc}") from exc

    if not result.success:
        return DiagnoseResponse(
            success=False,
            error_message=result.error_message,
            retries_used=result.retries_used,
        )

    # Step 3: 序列化输出
    nodes_data = [
        {
            "name": n.name,
            "definition": n.definition,
            "entity_type": n.entity_type,
            "status": n.status.value if hasattr(n.status, 'value') else str(n.status),
            "reason": n.reason,
            "source": n.source,
        }
        for n in result.output.nodes
    ]
    edges_data = [
        {
            "source": e.source,
            "target": e.target,
            "relation": e.relation.value if hasattr(e.relation, 'value') else str(e.relation),
            "status": e.status.value if hasattr(e.status, 'value') else str(e.status),
            "reason": e.reason,
        }
        for e in result.output.edges
    ]

    return DiagnoseResponse(
        success=True,
        nodes=nodes_data,
        edges=edges_data,
        summary=result.output.summary,
        retries_used=result.retries_used,
    )


@router.post("/chat", response_model=ChatResponse)
def chat(payload: ChatRequest) -> ChatResponse:
    """
    AI 对话端点（ReAct Agent 版）

    使用 LangChain ReAct Agent，具备：
    - 读取/编辑 Markdown 文档
    - 搜索知识图谱
    - 图像识别（多模态）

    Agent 会自主决策何时调用工具来完成任务。
    """
    if not payload.conversation_id:
        payload.conversation_id = "default"

    # 解析图谱数据
    try:
        graph_nodes = json.loads(payload.graph_nodes) if payload.graph_nodes else []
        graph_edges = json.loads(payload.graph_edges) if payload.graph_edges else []
    except json.JSONDecodeError:
        graph_nodes, graph_edges = [], []

    agent = get_react_chat_agent()

    try:
        result = agent.chat(
            user_message=payload.user_message,
            conversation_id=payload.conversation_id,
            markdown=payload.markdown,
            graph_nodes=graph_nodes,
            graph_edges=graph_edges,
            image_base64=payload.image_base64,
        )
    except Exception as exc:
        logger.exception("[LangChain Router] ReAct Agent 调用失败")
        raise HTTPException(status_code=502, detail=f"AI Agent 调用失败: {exc}") from exc

    return ChatResponse(
        reply=result["reply"],
        conversation_id=payload.conversation_id,
        edited_markdown=result.get("edited_markdown", ""),
    )


@router.post("/learning-path", response_model=LearningPathResponse)
def learning_path(payload: LearningPathRequest) -> LearningPathResponse:
    """
    学习路径指导端点

    结合 DFS 逆向依赖树 + 图谱节点状态信息，
    调用 LangChain Agent 生成个性化的学习路线指导文本。
    """
    # 格式化依赖树
    try:
        dependency_data = json.loads(payload.dependency_tree_json)
    except json.JSONDecodeError:
        dependency_data = []

    dep_tree_text = format_dependency_tree(
        concept_name=payload.target_concept,
        dependency_tree=dependency_data,
    )

    # 格式化节点状态
    all_nodes_text = format_graph_context(
        nodes_json=payload.graph_nodes_json,
        edges_json="[]",
    )

    # 组装 Prompt
    path_prompt = get_learning_path_prompt()
    llm = _build_chat_llm(temperature=0.3)

    messages = path_prompt.format_messages(
        target_concept=payload.target_concept,
        dependency_tree_json=dep_tree_text,
        all_related_nodes_json=all_nodes_text,
    )

    try:
        response = llm.invoke(messages)
        guidance = response.content if hasattr(response, 'content') else str(response)
        guidance = guidance.strip() if guidance else "无法生成学习路径。"
    except Exception as exc:
        logger.exception("[LangChain Router] 学习路径生成失败")
        raise HTTPException(status_code=502, detail=f"学习路径生成失败: {exc}") from exc

    return LearningPathResponse(guidance=guidance)


@router.get("/health")
def langchain_health() -> dict:
    """LangChain 模块健康检查"""
    llm_ok = bool(settings.deepseek_api_key.strip())
    return {
        "module": "langchain-agent",
        "status": "ok" if llm_ok else "no_api_key",
        "deepseek_model": settings.deepseek_model,
        "agents": ["ner", "fact_check", "supplement"],
        "endpoints": [
            "POST /api/langchain/diagnose",
            "POST /api/langchain/chat",
            "POST /api/langchain/learning-path",
            "GET  /api/langchain/health",
        ],
    }
