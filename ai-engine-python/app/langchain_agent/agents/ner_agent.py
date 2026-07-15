"""
NER 实体识别 Agent

使用 LangChain 框架，基于 ReAct 模式从 Markdown 笔记中自动抽取：
1. 专业概念实体（名称 + 定义 + 类型）
2. 实体间语义关系（前置依赖 / 从属 / 关联）

技术链路：ChatDeepSeek(LLM) + PromptTemplate + PydanticOutputParser
"""

import json
import logging
from typing import Optional, TYPE_CHECKING

if TYPE_CHECKING:
    from langchain_openai import ChatOpenAI

from app.core.config import settings
from app.langchain_agent.prompts import get_ner_prompt
from app.langchain_agent.schemas import NEROutput, ExtractedEntity, ExtractedRelation

logger = logging.getLogger(__name__)


def _build_llm(temperature: float = 0.2) -> "ChatOpenAI":
    # 延迟导入，确保未安装 langchain 时 schema/tools 层仍可正常使用
    from langchain_openai import ChatOpenAI  # noqa: PLC0415
    """
    构建 LangChain ChatOpenAI 实例（兼容 DeepSeek API）。

    DeepSeek API 与 OpenAI SDK 完全兼容，因此使用 langchain-openai 的
    ChatOpenAI 类，通过 base_url 指向 DeepSeek 端点。
    """
    return ChatOpenAI(
        model=settings.deepseek_model,
        api_key=settings.deepseek_api_key,
        base_url=settings.deepseek_base_url,
        temperature=temperature,
        max_tokens=4096,
        timeout=120,
        max_retries=2,
    )


class NERAgent:
    """
    NER（命名实体识别）Agent

    职责：
    - 从 Markdown 的 Semantic Chunk 中提取专业概念
    - 识别概念之间的知识图谱关系
    - 输出结构化的 NEROutput（entities + relations）

    使用方式：
        agent = NERAgent()
        result: NEROutput = agent.extract(chunks=["chunk1", "chunk2"])
    """

    def __init__(self, temperature: float = 0.2):
        self.llm = _build_llm(temperature=temperature)
        # 延迟导入：LangChain 依赖仅在 Agent 实际使用时才加载
        from langchain_core.output_parsers import PydanticOutputParser  # noqa: PLC0415
        self.parser = PydanticOutputParser(pydantic_object=NEROutput)
        self.prompt = get_ner_prompt()

    def extract(self, chunks: list[str]) -> NEROutput:
        """
        从 Markdown chunks 中抽取实体与关系。

        Args:
            chunks: Semantic Chunking 切出的文本块列表

        Returns:
            NEROutput: 包含 entities 和 relations 的结构化输出

        Raises:
            RuntimeError: LLM 调用失败或输出解析失败
        """
        from langchain_core.exceptions import OutputParserException  # noqa: PLC0415
        if not chunks or not any(c.strip() for c in chunks):
            raise ValueError("chunks 为空，无法进行实体抽取")

        # 拼接所有 chunk，用分隔符区分来源段落
        markdown_chunks = "\n\n".join(
            f"### 段落 {i + 1}\n{chunk}" for i, chunk in enumerate(chunks)
        )

        # 组装 Chain：Prompt -> LLM -> Parser
        chain = self.prompt | self.llm | self.parser

        try:
            result: NEROutput = chain.invoke({
                "markdown_chunks": markdown_chunks,
                "format_instructions": self.parser.get_format_instructions(),
            })
        except OutputParserException as exc:
            logger.error("NER Agent 输出解析失败: %s", exc)
            raise RuntimeError(f"实体抽取 Agent 输出格式异常，解析失败: {exc}") from exc
        except Exception as exc:
            logger.error("NER Agent LLM 调用异常: %s", exc)
            raise RuntimeError(f"实体抽取 Agent 调用失败: {exc}") from exc

        # 后处理：去重同名实体（保留定义更完整的那条）
        result = self._deduplicate_entities(result)

        logger.info(
            "NER Agent 抽取完成: %d 个实体, %d 条关系",
            len(result.entities), len(result.relations),
        )
        return result

    @staticmethod
    def _deduplicate_entities(output: NEROutput) -> NEROutput:
        """
        去重同名实体：若多个 chunk 抽到同一概念名，
        保留 definition 非空且较长的版本。
        """
        seen: dict[str, ExtractedEntity] = {}
        for entity in output.entities:
            name = entity.name.strip().lower()
            if name not in seen:
                seen[name] = entity
            else:
                # 保留定义更完整（更长）的版本
                existing_def = seen[name].definition or ""
                new_def = entity.definition or ""
                if len(new_def) > len(existing_def):
                    seen[name] = entity

        output.entities = list(seen.values())
        return output

    async def aextract(self, chunks: list[str]) -> NEROutput:
        """异步版本的实体抽取（用于高并发场景）"""
        from langchain_core.exceptions import OutputParserException  # noqa: PLC0415
        markdown_chunks = "\n\n".join(
            f"### 段落 {i + 1}\n{chunk}" for i, chunk in enumerate(chunks)
        )
        chain = self.prompt | self.llm | self.parser

        try:
            result: NEROutput = await chain.ainvoke({
                "markdown_chunks": markdown_chunks,
                "format_instructions": self.parser.get_format_instructions(),
            })
        except OutputParserException as exc:
            logger.error("NER Agent 异步输出解析失败: %s", exc)
            raise RuntimeError(f"实体抽取 Agent 输出格式异常: {exc}") from exc
        except Exception as exc:
            logger.error("NER Agent 异步 LLM 调用异常: %s", exc)
            raise RuntimeError(f"实体抽取 Agent 调用失败: {exc}") from exc

        result = self._deduplicate_entities(result)
        logger.info("NER Agent 异步抽取完成: %d 个实体, %d 条关系", len(result.entities), len(result.relations))
        return result


# 模块级单例（避免重复初始化 LLM 连接）
_ner_agent: Optional[NERAgent] = None


def get_ner_agent() -> NERAgent:
    """获取 NER Agent 单例"""
    global _ner_agent
    if _ner_agent is None:
        _ner_agent = NERAgent()
    return _ner_agent
