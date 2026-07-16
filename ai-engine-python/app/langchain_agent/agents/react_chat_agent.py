"""
ReAct 对话 Agent

基于 LangChain ReAct (Reasoning + Acting) 模式的智能对话 Agent。
与旧的简单 ChatPromptTemplate 不同，此 Agent 拥有工具调用能力：
- read_current_markdown: 读取文档
- edit_markdown: 精确修改文档
- append_markdown_section: 追加章节
- search_knowledge_graph: 搜索知识图谱

Agent 会自主决策何时调用工具、如何组合工具完成任务。
"""

import logging
import re
from typing import Optional

from langchain_openai import ChatOpenAI
from langchain_core.messages import HumanMessage, SystemMessage
from langgraph.prebuilt import create_react_agent

from app.core.config import settings
from app.langchain_agent.tools.agent_tools import (
    get_agent_tools,
    set_document,
    get_document,
    pop_edited_markdown,
)

logger = logging.getLogger(__name__)

SYSTEM_PROMPT = """你是一位专业的「AI 学习导师兼文档编辑助手」。

## 你的能力
你可以访问学生当前正在学习的 Markdown 笔记和知识图谱，并拥有以下工具：
1. **read_current_markdown** — 读取笔记全文，了解学生写了什么
2. **edit_markdown** — 精确替换文档中的错误内容
3. **append_markdown_section** — 在文档末尾补充新知识点
4. **search_knowledge_graph** — 搜索知识图谱中的概念及关系

## 工作流程
当学生提出要求时，请按以下步骤操作：
1. 先 **read_current_markdown** 了解笔记现状
2. 需要时 **search_knowledge_graph** 查看图谱中的概念状态（error/supplement/correct）
3. 根据发现的问题：
   - 如有事实错误 → **edit_markdown** 替换错误内容
   - 如有知识缺口 → **append_markdown_section** 补充内容
4. 最后用通俗语言向学生解释你做了什么、为什么这样做

## 文档修改规则
- 修改文档时必须使用 edit_markdown 或 append_markdown_section 工具
- 不要虚构不存在的概念，补全内容必须有学科依据
- 修改后简要告知学生改变了什么
- 如果学生要求添加例题/练习，使用 append_markdown_section

## 对话风格
- 先诊断再下药：先读文档、查图谱，再给出修改建议
- 解释要通俗易懂，必要时给出类比
- 对 error 节点（笔记错误）要明确指出错在哪里
- 对 supplement 节点（知识缺口）要解释为什么需要补全
- 每次交互后，主动建议 1-2 条下一步学习方向

## 图像理解
如果学生上传了图片，请分析图片内容（公式、图表、手写笔记等），
将其转化为 Markdown 格式的笔记内容，并使用 append_markdown_section 追加到文档中。
如果没有图片，忽略此条。
"""


def _build_llm(temperature: float = 0.5) -> ChatOpenAI:
    """构建 ChatOpenAI 实例（DeepSeek 兼容）"""
    return ChatOpenAI(
        model=settings.deepseek_model,
        api_key=settings.deepseek_api_key,
        base_url=settings.deepseek_base_url,
        temperature=temperature,
        max_tokens=4096,
        timeout=120,
        max_retries=2,
    )


class ReactChatAgent:
    """ReAct 对话 Agent — 具备工具调用能力的 AI 学习导师"""

    def __init__(self, temperature: float = 0.5):
        self.llm = _build_llm(temperature=temperature)
        self.tools = get_agent_tools()
        # 使用 langgraph 的预构建 ReAct agent
        self.agent = create_react_agent(
            model=self.llm,
            tools=self.tools,
            prompt=SYSTEM_PROMPT,
        )

    def chat(
        self,
        user_message: str,
        conversation_id: str,
        markdown: str = "",
        graph_nodes: list[dict] | None = None,
        graph_edges: list[dict] | None = None,
        image_base64: str = "",
    ) -> dict:
        """
        执行一次对话交互。

        Args:
            user_message: 用户输入的消息
            conversation_id: 对话会话 ID
            markdown: 当前 Markdown 笔记全文
            graph_nodes: 图谱节点列表
            graph_edges: 图谱边列表
            image_base64: 图片的 Base64 编码（可选）

        Returns:
            {"reply": "AI 回复文本", "edited_markdown": "修改后的 MD（若有修改）"}
        """
        # 注册当前会话的文档
        if markdown.strip() or graph_nodes:
            set_document(
                conversation_id=conversation_id,
                markdown=markdown,
                graph_nodes=graph_nodes or [],
                graph_edges=graph_edges or [],
            )

        # 构建用户消息
        message_content = user_message
        if image_base64:
            # 多模态：附加图片
            message_content = [
                {"type": "text", "text": user_message or "请分析这张图片，将其内容转化为 Markdown 格式的笔记。"},
                {"type": "image_url", "image_url": {"url": f"data:image/png;base64,{image_base64}"}},
            ]

        # 构建消息（conversation_id 注入到消息中以供工具使用）
        full_message = (
            f"[会话ID: {conversation_id}]\n\n{message_content}"
            if isinstance(message_content, str)
            else message_content
        )

        try:
            # 调用 ReAct Agent
            result = self.agent.invoke({
                "messages": [HumanMessage(content=full_message)],
            })

            # 提取最后一条 AI 消息
            messages = result.get("messages", [])
            reply = ""
            for msg in reversed(messages):
                if hasattr(msg, "content") and msg.type == "ai":
                    reply = msg.content or ""
                    break

            if not reply:
                reply = "抱歉，我暂时无法处理这个请求，请稍后再试。"

            # 去除工具调用的残留标记
            reply = self._clean_reply(reply)

        except Exception as exc:
            logger.exception("ReAct Agent 调用失败")
            reply = f"抱歉，处理请求时出错：{str(exc)[:200]}"

        # 检查文档是否被修改
        edited_md = pop_edited_markdown(conversation_id)

        return {
            "reply": reply,
            "edited_markdown": edited_md or "",
        }

    @staticmethod
    def _clean_reply(text: str) -> str:
        """清理 ReAct Agent 回复中的工具调用残留意向"""
        # 去除可能残留的 thought/action 标记
        text = re.sub(r'(?i)(thought|action|observation)\s*:', '', text)
        text = re.sub(r'```json\s*\{.*?\}\s*```', '', text, flags=re.DOTALL)
        # 去除首尾空白
        text = text.strip()
        return text


# 模块级单例
_react_chat_agent: Optional[ReactChatAgent] = None


def get_react_chat_agent() -> ReactChatAgent:
    """获取 ReAct Chat Agent 单例"""
    global _react_chat_agent
    if _react_chat_agent is None:
        _react_chat_agent = ReactChatAgent()
    return _react_chat_agent
