"""
知识补全 Agent (Supplement Agent)

基于 LangChain 框架，检测知识链路中的逻辑断层并自动生成补全节点。
当笔记中从概念 A 跳跃到概念 C 而缺失关键中间概念 B 时，Agent 会：
1. 识别逻辑断层
2. 推理缺失的前置知识
3. 生成 status=supplement 的补充节点及其定义
"""

import json
import logging
from typing import Optional, TYPE_CHECKING

if TYPE_CHECKING:
    from langchain_openai import ChatOpenAI

from app.core.config import settings
from app.langchain_agent.prompts import get_supplement_prompt
from app.langchain_agent.schemas import (
    NEROutput,
    SupplementOutput,
    SupplementItem,
)

logger = logging.getLogger(__name__)


def _build_llm(temperature: float = 0.3) -> "ChatOpenAI":
    """知识补全需要适中的 temperature（0.3）"""
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


class SupplementAgent:
    """
    知识补全 Agent（"课程体系架构师"）

    职责：
    - 分析 NER Agent 抽取的实体与关系链路
    - 检测逻辑跳跃和前置知识缺失
    - 生成 status=supplement 的补充节点（名称 + 定义 + 补全理由）

    使用方式：
        agent = SupplementAgent()
        result: SupplementOutput = agent.detect_gaps(ner_output, original_markdown)
    """

    def __init__(self, temperature: float = 0.3):
        self.llm = _build_llm(temperature=temperature)
        from langchain_core.output_parsers import PydanticOutputParser  # noqa: PLC0415
        self.parser = PydanticOutputParser(pydantic_object=SupplementOutput)
        self.prompt = get_supplement_prompt()

    def detect_gaps(
        self,
        ner_output: NEROutput,
        original_markdown: str,
    ) -> SupplementOutput:
        """
        检测知识链路中的逻辑断层并生成补全建议。

        Args:
            ner_output: NER Agent 的抽取结果（entities + relations）
            original_markdown: 原始 Markdown 笔记文本

        Returns:
            SupplementOutput: 补全项列表（若逻辑完整则为空列表）

        Raises:
            RuntimeError: LLM 调用失败或输出解析失败
        """
        from langchain_core.exceptions import OutputParserException  # noqa: PLC0415
        if not ner_output.entities:
            raise ValueError("NER 输出中无实体，无法进行逻辑断层检测")

        entities_json = json.dumps(
            [
                {"name": e.name, "definition": e.definition or "(无)", "type": e.entity_type}
                for e in ner_output.entities
            ],
            ensure_ascii=False,
            indent=2,
        )

        relations_json = json.dumps(
            [
                {
                    "source": r.source,
                    "target": r.target,
                    "relation": r.relation.value if hasattr(r.relation, 'value') else str(r.relation),
                    "evidence": r.evidence,
                }
                for r in ner_output.relations
            ],
            ensure_ascii=False,
            indent=2,
        ) if ner_output.relations else "（无关系数据）"

        chain = self.prompt | self.llm | self.parser

        try:
            result: SupplementOutput = chain.invoke({
                "original_markdown": original_markdown[:8000],
                "entities_json": entities_json,
                "relations_json": relations_json,
                "format_instructions": self.parser.get_format_instructions(),
            })
        except OutputParserException as exc:
            logger.error("Supplement Agent 输出解析失败: %s", exc)
            raise RuntimeError(f"知识补全 Agent 输出格式异常: {exc}") from exc
        except Exception as exc:
            logger.error("Supplement Agent LLM 调用异常: %s", exc)
            raise RuntimeError(f"知识补全 Agent 调用失败: {exc}") from exc

        # 验证补全项：不允许补全一个已经在 NER 输出中存在的概念
        self._validate_supplements(result.supplements, ner_output)

        logger.info(
            "Supplement Agent 检测完成: %d 个逻辑断层，生成 %d 个补全节点",
            len(result.supplements), len(result.supplements),
        )
        return result

    @staticmethod
    def _validate_supplements(
        supplements: list[SupplementItem],
        ner_output: NEROutput,
    ) -> None:
        """
        验证补全建议的合理性：
        - 补全的新概念不应与已有实体完全重名（否则无需补全）
        - 若出现重名，记录警告但不抛异常（可能是 LLM 对同一概念给出了更深入的视角）
        """
        existing_names = {e.name.strip().lower() for e in ner_output.entities}
        for sup in supplements:
            if sup.supplement_name.strip().lower() in existing_names:
                logger.warning(
                    "Supplement Agent 补全了已存在的概念 '%s'，"
                    "可能是笔记中该概念定义不完整，Agent 给出了补充视角",
                    sup.supplement_name,
                )

    async def adetect_gaps(
        self,
        ner_output: NEROutput,
        original_markdown: str,
    ) -> SupplementOutput:
        """异步版本的知识补全"""
        entities_json = json.dumps(
            [{"name": e.name, "definition": e.definition or "(无)", "type": e.entity_type}
             for e in ner_output.entities],
            ensure_ascii=False, indent=2,
        )
        relations_json = json.dumps(
            [{"source": r.source, "target": r.target,
              "relation": r.relation.value if hasattr(r.relation, 'value') else str(r.relation),
              "evidence": r.evidence}
             for r in ner_output.relations],
            ensure_ascii=False, indent=2,
        ) if ner_output.relations else "（无关系数据）"

        from langchain_core.exceptions import OutputParserException  # noqa: PLC0415
        chain = self.prompt | self.llm | self.parser

        try:
            result: SupplementOutput = await chain.ainvoke({
                "original_markdown": original_markdown[:8000],
                "entities_json": entities_json,
                "relations_json": relations_json,
                "format_instructions": self.parser.get_format_instructions(),
            })
        except OutputParserException as exc:
            logger.error("Supplement Agent 异步输出解析失败: %s", exc)
            raise RuntimeError(f"知识补全 Agent 输出格式异常: {exc}") from exc
        except Exception as exc:
            logger.error("Supplement Agent 异步 LLM 调用异常: %s", exc)
            raise RuntimeError(f"知识补全 Agent 调用失败: {exc}") from exc

        self._validate_supplements(result.supplements, ner_output)
        logger.info("Supplement Agent 异步检测完成: %d 个补全节点", len(result.supplements))
        return result


# 模块级单例
_supplement_agent: Optional[SupplementAgent] = None


def get_supplement_agent() -> SupplementAgent:
    """获取 Supplement Agent 单例"""
    global _supplement_agent
    if _supplement_agent is None:
        _supplement_agent = SupplementAgent()
    return _supplement_agent
