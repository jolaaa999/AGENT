"""
事实校验 Agent (Fact-Check Agent)

基于 LangChain Chain-of-Thought（CoT）实现"审稿人"角色 Agent。
对 NER Agent 抽取的实体逐条进行事实核查，检测笔记中的概念性错误。

核心流程：
1. 接收 NER 输出 + 原始笔记
2. CoT 推理：理解断言 -> 调取先验 -> 逐项对比 -> 判定
3. 对错误节点输出 status=error + reason（纠错理由）+ corrected_definition
"""

import json
import logging
from typing import Optional, TYPE_CHECKING

if TYPE_CHECKING:
    from langchain_openai import ChatOpenAI

from app.core.config import settings
from app.langchain_agent.prompts import get_fact_check_prompt
from app.langchain_agent.schemas import (
    FactCheckOutput,
    FactCheckItem,
    NEROutput,
    NodeStatus,
)

logger = logging.getLogger(__name__)


def _build_llm(temperature: float = 0.15) -> "ChatOpenAI":
    """事实校验需要更低的 temperature（0.15）"""
    from langchain_openai import ChatOpenAI  # noqa: PLC0415
    return ChatOpenAI(
        model=settings.deepseek_model,
        api_key=settings.deepseek_api_key,
        base_url=settings.deepseek_base_url,
        temperature=temperature,
        max_tokens=4096,
        timeout=120,
        max_retries=2,
    )


class FactCheckAgent:
    """
    事实校验 Agent（"学术审稿人"）

    职责：
    - 逐条审查 NER Agent 抽取的实体
    - 基于 LLM 先验知识判定概念描述是否正确
    - 对错误节点给出详细的纠错理由和正确描述
    - 不修改用户原文，仅在输出中标记 status=error

    使用方式：
        agent = FactCheckAgent()
        result: FactCheckOutput = agent.verify(ner_output, original_markdown)
    """

    def __init__(self, temperature: float = 0.15):
        self.llm = _build_llm(temperature=temperature)
        from langchain_core.output_parsers import PydanticOutputParser  # noqa: PLC0415
        self.parser = PydanticOutputParser(pydantic_object=FactCheckOutput)
        self.prompt = get_fact_check_prompt()

    def verify(
        self,
        ner_output: NEROutput,
        original_markdown: str,
    ) -> FactCheckOutput:
        """
        对 NER 抽取结果进行事实校验。

        Args:
            ner_output: NER Agent 的抽取结果（entities + relations）
            original_markdown: 原始 Markdown 笔记文本（用于上下文参照）

        Returns:
            FactCheckOutput: 逐条校验结果列表

        Raises:
            RuntimeError: LLM 调用失败或输出解析失败
        """
        if not ner_output.entities:
            raise ValueError("NER 输出中无实体，无需进行事实校验")

        # 将实体序列化为 JSON 供 LLM 审查
        entities_json = json.dumps(
            [
                {
                    "name": e.name,
                    "definition": e.definition or "(笔记未定义)",
                    "type": e.entity_type,
                }
                for e in ner_output.entities
            ],
            ensure_ascii=False,
            indent=2,
        )

        from langchain_core.exceptions import OutputParserException  # noqa: PLC0415
        chain = self.prompt | self.llm | self.parser

        try:
            result: FactCheckOutput = chain.invoke({
                "original_markdown": original_markdown[:8000],  # 截断过长文本，保留足够上下文
                "entities_json": entities_json,
                "format_instructions": self.parser.get_format_instructions(),
            })
        except OutputParserException as exc:
            logger.error("FactCheck Agent 输出解析失败: %s", exc)
            raise RuntimeError(f"事实校验 Agent 输出格式异常: {exc}") from exc
        except Exception as exc:
            logger.error("FactCheck Agent LLM 调用异常: %s", exc)
            raise RuntimeError(f"事实校验 Agent 调用失败: {exc}") from exc

        # 统计校验结果
        error_count = sum(1 for c in result.checks if c.status == NodeStatus.error)
        correct_count = sum(1 for c in result.checks if c.status == NodeStatus.correct)

        logger.info(
            "FactCheck Agent 校验完成: %d 条审查, %d correct, %d error",
            len(result.checks), correct_count, error_count,
        )

        # 校验结果合理性检查
        self._validate_checks(result.checks, ner_output)

        return result

    @staticmethod
    def _validate_checks(checks: list[FactCheckItem], ner_output: NEROutput) -> None:
        """
        校验结果的合理性检查：
        - 确保返回的 entity_name 都能在 NER 输出中找到对应（防止 LLM 幻觉编造）
        - 若出现不匹配，仅记录警告而不抛异常（允许 LLM 合并/拆分实体的合理行为）
        """
        ner_names = {e.name.strip().lower() for e in ner_output.entities}
        for check in checks:
            if check.entity_name.strip().lower() not in ner_names:
                logger.warning(
                    "FactCheck 返回了 NER 输出中不存在的实体 '%s'，可能是 LLM 拆分/合并了实体",
                    check.entity_name,
                )

    async def averify(
        self,
        ner_output: NEROutput,
        original_markdown: str,
    ) -> FactCheckOutput:
        """异步版本的事实校验"""
        entities_json = json.dumps(
            [
                {"name": e.name, "definition": e.definition or "(笔记未定义)", "type": e.entity_type}
                for e in ner_output.entities
            ],
            ensure_ascii=False,
            indent=2,
        )

        from langchain_core.exceptions import OutputParserException  # noqa: PLC0415
        chain = self.prompt | self.llm | self.parser

        try:
            result: FactCheckOutput = await chain.ainvoke({
                "original_markdown": original_markdown[:8000],
                "entities_json": entities_json,
                "format_instructions": self.parser.get_format_instructions(),
            })
        except OutputParserException as exc:
            logger.error("FactCheck Agent 异步输出解析失败: %s", exc)
            raise RuntimeError(f"事实校验 Agent 输出格式异常: {exc}") from exc
        except Exception as exc:
            logger.error("FactCheck Agent 异步 LLM 调用异常: %s", exc)
            raise RuntimeError(f"事实校验 Agent 调用失败: {exc}") from exc

        error_count = sum(1 for c in result.checks if c.status == NodeStatus.error)
        correct_count = sum(1 for c in result.checks if c.status == NodeStatus.correct)
        logger.info("FactCheck Agent 异步校验完成: %d 条审查, %d correct, %d error",
                    len(result.checks), correct_count, error_count)

        self._validate_checks(result.checks, ner_output)
        return result


# 模块级单例
_fact_check_agent: Optional[FactCheckAgent] = None


def get_fact_check_agent() -> FactCheckAgent:
    """获取 FactCheck Agent 单例"""
    global _fact_check_agent
    if _fact_check_agent is None:
        _fact_check_agent = FactCheckAgent()
    return _fact_check_agent
