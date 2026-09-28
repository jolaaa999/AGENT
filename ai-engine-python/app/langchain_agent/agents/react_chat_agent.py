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

from app.core.llm import build_chat_openai
from app.langchain_agent.tools.agent_tools import (
    get_agent_tools,
    set_document,
    get_document,
    pop_edited_markdown,
    set_active_conversation,
)
from app.langchain_agent.tools.workspace_tools import (
    get_workspace_tools,
    set_workspace_user,
)

logger = logging.getLogger(__name__)

SYSTEM_PROMPT = """你是一位专业的「AI 学习导师兼文档编辑助手」。

## 你的能力
你有一个**专属工作区**（沙箱），里面放着学生上传的学习资料，
你可以像操作真实文件一样读写它们。可用工具分为两组：

### A. 工作区文件工具（操作真实文件，结果可被学生下载）
1. **workspace_list_files** — 列出工作区里有哪些文件
2. **workspace_read_file** — 读取某个文件全文
3. **workspace_write_file** — 把改好的内容写回文件（会真正保存）
4. **workspace_search** — 在所有文件里搜索关键字，定位知识点
5. **workspace_rename_file** — 重命名文件

### B. 当前笔记工具（编辑界面里正在看的那一份）
6. **read_current_markdown** — 读取当前笔记全文
7. **edit_markdown** — 精确替换当前笔记中的错误内容
8. **append_markdown_section** — 在当前笔记末尾补充新知识点
9. **search_knowledge_graph** — 搜索知识图谱中的概念及关系

## 何时用哪一组（先判断再动手）
- 学生说「我的文件 / 工作区 / 批量修改 / 改完要下载」→ 用 **A 组工作区工具**，
  第一步永远先调 **workspace_list_files** 看清有哪些文件
- 学生只是针对界面里当前这篇笔记提问 → 用 **B 组笔记工具**
- 判断不了时：先调 **workspace_list_files**。
  只要工作区里有文件，就应该用 A 组，而不是 B 组。
- 注意：B 组的 read_current_markdown 读不到工作区文件，
  如果它返回「当前没有打开的文档」，说明你选错了工具组，请改用 A 组。

## 安全与边界（重要）
- 你只能操作工作区里的文件，不要尝试访问工作区之外的任何路径
- 写入前必须先读取原内容，基于全文改写；不要凭记忆重建，避免丢失学生原有信息
- 无法执行任意系统命令（这是有意的限制），需要什么请用文件工具完成
- 不要虚构不存在的概念，所有补充必须有学科依据

## 工作流程
1. 先弄清楚范围：列表 / 搜索定位相关文件
2. 读取相关文件，并结合 **search_knowledge_graph** 看概念状态（error/supplement/correct）
3. 根据发现的问题动手修改：
   - 事实错误 → 改写错误段落
   - 知识缺口 → 补充内容
4. 修改后，明确告诉学生「改了哪个文件、改了什么」，
   并提示他们可以在界面上直接下载修改后的文件

## 对话风格
- 先诊断再下药：先读文件、查图谱，再给结论
- 解释要通俗易懂，必要时给出类比
- 对 error 节点（笔记错误）要指出错在哪里
- 对 supplement 节点（知识缺口）要解释为什么需要补全
- 每次交互后，主动建议 1-2 条下一步学习方向

## 图像理解
如果学生上传了图片，请分析图片内容（公式、图表、手写笔记等），
将其转化为 Markdown 笔记，并追加到文档中。如果没有图片，忽略此条。
"""


def _build_llm(temperature: float = 0.5) -> ChatOpenAI:
    """构建 ChatOpenAI 实例（DeepSeek 兼容）"""
    return build_chat_openai(temperature=temperature)


class ReactChatAgent:
    """ReAct 对话 Agent — 具备工具调用能力的 AI 学习导师"""

    def __init__(self, temperature: float = 0.5):
        self.llm = _build_llm(temperature=temperature)
        self.tools = get_agent_tools() + get_workspace_tools()
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
        user_id: str = "",
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
            user_id: 当前用户 ID（用于定位其专属工作区）

        Returns:
            {"reply": "AI 回复文本", "edited_markdown": "修改后的 MD（若有修改）"}
        """
        # 绑定用户 -> 会话，供工作区文件工具定位沙箱目录。
        # 用户身份由后端传入而非由 LLM 生成，避免被提示词篡改。
        # 同时标记「活跃会话」，这样工具无需依赖 LLM 回填 conversation_id。
        set_active_conversation(conversation_id)
        if user_id:
            set_workspace_user(conversation_id, user_id)

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

        # 构建用户消息。
        # 注意：会话ID 现在通过服务端状态（set_active_conversation）提供给工具，
        # 不再依赖在消息里拼字符串——那样在「图文混合」场景下会因为 content 是
        # 列表而丢失前缀，导致工具拿不到会话上下文。
        full_message = message_content

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
