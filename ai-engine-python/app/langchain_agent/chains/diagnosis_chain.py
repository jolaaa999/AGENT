"""
诊断链 (Diagnosis Chain)

编排三个 LangChain Agent 的顺序执行流水线：
  NER Agent（抽取实体+关系）
  -> FactCheck Agent（事实校验，标记 error）
  -> Supplement Agent（检测逻辑断层，生成 supplement 补全节点）

最终将所有 Agent 输出聚合为 DiagnosisOutput，
其格式直接对应 Go 后端入库所需的 nodes[] + edges[] 结构。

处理流程还包含：
- JSON 格式断言与校验
- 失败时的降级重试策略
- 对各 Agent 输出的合理性交叉验证
"""

import json
import logging
from dataclasses import dataclass, field

from app.langchain_agent.agents import (
    get_ner_agent,
    get_fact_check_agent,
    get_supplement_agent,
)
from app.langchain_agent.schemas import (
    DiagnosisEdge,
    DiagnosisNode,
    DiagnosisOutput,
    FactCheckOutput,
    NEROutput,
    NodeStatus,
    RelationType,
    SupplementOutput,
)

logger = logging.getLogger(__name__)


@dataclass
class DiagnosisResult:
    """诊断流水线的完整结果"""
    success: bool = False
    output: DiagnosisOutput = field(default_factory=lambda: DiagnosisOutput(nodes=[], edges=[]))
    error_message: str = ""
    retries_used: int = 0
    # 各 Agent 的原始输出（便于调试和日志追溯）
    ner_output: NEROutput | None = None
    fact_check_output: FactCheckOutput | None = None
    supplement_output: SupplementOutput | None = None


class DiagnosisChain:
    """
    诊断流水线

    将三个 LangChain Agent 串联为一个完整的诊断流程。
    从 Markdown 文本到最终图谱数据的端到端转换。

    使用方式：
        chain = DiagnosisChain()
        result = chain.run(chunks=["chunk1", "chunk2"], original_markdown="# Title\n...")
        if result.success:
            print(result.output.nodes)
            print(result.output.edges)
    """

    def __init__(self, max_retries: int = 2):
        self.max_retries = max_retries
        self.ner_agent = get_ner_agent()
        self.fact_check_agent = get_fact_check_agent()
        self.supplement_agent = get_supplement_agent()

    def run(
        self,
        chunks: list[str],
        original_markdown: str = "",
    ) -> DiagnosisResult:
        """
        执行完整的诊断流水线。

        Args:
            chunks: AST Semantic Chunking 切分的文本块
            original_markdown: 原始 Markdown 全文（用于事实校验和补全的上下文参照）

        Returns:
            DiagnosisResult: 包含成功标志、聚合后的图谱数据、以及各 Agent 原始输出的结果对象
        """
        if not chunks or not any(c.strip() for c in chunks):
            return DiagnosisResult(
                success=False,
                error_message="输入 chunks 为空，无法执行诊断流水线",
            )

        markdown_text = original_markdown or "\n\n".join(chunks)
        result = DiagnosisResult()
        retries = 0

        # ========== Stage 1: NER 实体抽取 ==========
        logger.info("[DiagnosisChain] Stage 1/3: NER 实体抽取开始")
        for attempt in range(self.max_retries + 1):
            try:
                ner_output = self.ner_agent.extract(chunks)
                result.ner_output = ner_output
                logger.info(
                    "[DiagnosisChain] Stage 1 完成: %d 实体, %d 关系",
                    len(ner_output.entities), len(ner_output.relations),
                )
                break
            except Exception as exc:
                logger.warning("[DiagnosisChain] NER 第 %d 次尝试失败: %s", attempt + 1, exc)
                retries += 1
                if attempt == self.max_retries:
                    return DiagnosisResult(
                        success=False,
                        error_message=f"NER 实体抽取在 {self.max_retries + 1} 次重试后仍失败: {exc}",
                        retries_used=retries,
                    )
        else:
            return DiagnosisResult(
                success=False,
                error_message="NER 实体抽取失败（未知原因）",
                retries_used=retries,
            )

        # ========== Stage 2: 事实校验 ==========
        logger.info("[DiagnosisChain] Stage 2/3: 事实校验开始")
        fact_check_output = None
        for attempt in range(self.max_retries + 1):
            try:
                fact_check_output = self.fact_check_agent.verify(ner_output, markdown_text)
                result.fact_check_output = fact_check_output
                error_count = sum(1 for c in fact_check_output.checks if c.status == NodeStatus.error)
                logger.info(
                    "[DiagnosisChain] Stage 2 完成: %d 条审查, %d error",
                    len(fact_check_output.checks), error_count,
                )
                break
            except Exception as exc:
                logger.warning("[DiagnosisChain] FactCheck 第 %d 次尝试失败: %s", attempt + 1, exc)
                retries += 1
                if attempt == self.max_retries:
                    # 事实校验失败时降级：将所有实体标记为 correct，不阻塞流水线
                    logger.warning("[DiagnosisChain] FactCheck 全部重试失败，降级为全部 correct")
                    fact_check_output = None
                    break

        # ========== Stage 3: 知识补全 ==========
        logger.info("[DiagnosisChain] Stage 3/3: 知识补全开始")
        supplement_output = None
        for attempt in range(self.max_retries + 1):
            try:
                supplement_output = self.supplement_agent.detect_gaps(ner_output, markdown_text)
                result.supplement_output = supplement_output
                logger.info(
                    "[DiagnosisChain] Stage 3 完成: %d 个补全节点",
                    len(supplement_output.supplements),
                )
                break
            except Exception as exc:
                logger.warning("[DiagnosisChain] Supplement 第 %d 次尝试失败: %s", attempt + 1, exc)
                retries += 1
                if attempt == self.max_retries:
                    logger.warning("[DiagnosisChain] Supplement 全部重试失败，跳过补全阶段")
                    supplement_output = None
                    break

        # ========== 聚合：合并三个 Agent 输出为 DiagnosisOutput ==========
        result.output = self._aggregate(
            ner_output=ner_output,
            fact_check_output=fact_check_output,
            supplement_output=supplement_output,
        )
        result.success = True
        result.retries_used = retries

        logger.info(
            "[DiagnosisChain] 全部完成: %d nodes, %d edges, %d retries",
            len(result.output.nodes), len(result.output.edges), retries,
        )
        return result

    def _aggregate(
        self,
        ner_output: NEROutput,
        fact_check_output: FactCheckOutput | None,
        supplement_output: SupplementOutput | None,
    ) -> DiagnosisOutput:
        """
        聚合三个 Agent 的输出为统一的 DiagnosisOutput。

        聚合规则：
        1. NER 的 entities -> DiagnosisNode（status=correct 为默认值）
        2. FactCheck 的结果覆盖对应实体的 status 和 reason
        3. Supplement 的结果追加为新的 DiagnosisNode（status=supplement）
        4. NER 的 relations -> DiagnosisEdge
        5. 所有输出按名称去重
        """
        nodes: dict[str, DiagnosisNode] = {}  # key=name.lower()

        # 第1步：NER 实体 -> 节点（默认 status=correct）
        for entity in ner_output.entities:
            key = entity.name.strip().lower()
            nodes[key] = DiagnosisNode(
                name=entity.name.strip(),
                definition=entity.definition,
                entity_type=entity.entity_type,
                status=NodeStatus.correct,
                reason="",
                source="ner",
            )

        # 第2步：FactCheck 结果覆盖 status
        if fact_check_output:
            for check in fact_check_output.checks:
                key = check.entity_name.strip().lower()
                if key in nodes:
                    nodes[key].status = check.status
                    nodes[key].reason = check.reason
                    if check.status == NodeStatus.error and check.corrected_definition:
                        # 错误节点：用纠正后的定义覆盖
                        nodes[key].definition = check.corrected_definition
                    nodes[key].source = "fact_check"
                else:
                    # FactCheck 提到了 NER 输出中没有的实体（可能是分解/合并）
                    nodes[key] = DiagnosisNode(
                        name=check.entity_name.strip(),
                        definition=check.corrected_definition,
                        entity_type="concept",
                        status=check.status,
                        reason=check.reason,
                        source="fact_check",
                    )

        # 第3步：Supplement 补全追加新节点
        if supplement_output:
            for sup in supplement_output.supplements:
                key = sup.supplement_name.strip().lower()
                if key not in nodes:
                    nodes[key] = DiagnosisNode(
                        name=sup.supplement_name.strip(),
                        definition=sup.supplement_definition,
                        entity_type="concept",
                        status=NodeStatus.supplement,
                        reason=sup.reason,
                        source="supplement",
                    )

        # 第4步：NER 关系 -> 边
        edge_set: set[tuple[str, str, str]] = set()  # (source_lower, target_lower, relation)
        edges: list[DiagnosisEdge] = []
        for rel in ner_output.relations:
            key_tuple = (
                rel.source.strip().lower(),
                rel.target.strip().lower(),
                rel.relation.value if hasattr(rel.relation, 'value') else str(rel.relation),
            )
            if key_tuple not in edge_set:
                edge_set.add(key_tuple)
                edges.append(DiagnosisEdge(
                    source=rel.source.strip(),
                    target=rel.target.strip(),
                    relation=rel.relation if isinstance(rel.relation, RelationType) else RelationType.RELATED_TO,
                    status=NodeStatus.correct,
                    reason=rel.evidence,
                    source_agent="ner",
                ))

        # 第5步：补全关系（supplement 节点与目标实体的 SUPPLEMENTS 关系）
        if supplement_output:
            for sup in supplement_output.supplements:
                key_tuple = (
                    sup.supplement_name.strip().lower(),
                    sup.target_entity.strip().lower(),
                    "SUPPLEMENTS",
                )
                if key_tuple not in edge_set:
                    edge_set.add(key_tuple)
                    edges.append(DiagnosisEdge(
                        source=sup.supplement_name.strip(),
                        target=sup.target_entity.strip(),
                        relation=RelationType.SUPPLEMENTS,
                        status=NodeStatus.supplement,
                        reason=sup.reason,
                        source_agent="supplement",
                    ))

        # 第6步：错误纠正关系（error 节点与原始实体的 CORRECTS 关系）
        if fact_check_output:
            for check in fact_check_output.checks:
                if check.status == NodeStatus.error:
                    key_tuple = (
                        check.entity_name.strip().lower(),
                        check.entity_name.strip().lower(),
                        "CORRECTS",
                    )
                    # error 节点的关系——实际上 error 节点本身就是被标记的原实体，
                    # 不需要额外添加 CORRECTS 边，status 字段已足够表达
                    pass

        # 生成 summary
        error_count = sum(1 for n in nodes.values() if n.status == NodeStatus.error)
        supplement_count = sum(1 for n in nodes.values() if n.status == NodeStatus.supplement)
        correct_count = sum(1 for n in nodes.values() if n.status == NodeStatus.correct)

        summary_parts = [f"诊断完成：共 {len(nodes)} 个节点, {len(edges)} 条边"]
        if error_count > 0:
            summary_parts.append(f"{error_count} 个错误节点（需纠正）")
        if supplement_count > 0:
            summary_parts.append(f"{supplement_count} 个AI补全节点（逻辑断层）")
        summary_parts.append(f"{correct_count} 个正常节点")

        return DiagnosisOutput(
            nodes=list(nodes.values()),
            edges=edges,
            summary="；".join(summary_parts),
        )

    async def arun(
        self,
        chunks: list[str],
        original_markdown: str = "",
    ) -> DiagnosisResult:
        """异步版本的诊断流水线"""
        if not chunks or not any(c.strip() for c in chunks):
            return DiagnosisResult(success=False, error_message="输入 chunks 为空")

        markdown_text = original_markdown or "\n\n".join(chunks)
        result = DiagnosisResult()
        retries = 0

        # Stage 1: NER
        logger.info("[DiagnosisChain Async] Stage 1/3: NER")
        for attempt in range(self.max_retries + 1):
            try:
                ner_output = await self.ner_agent.aextract(chunks)
                result.ner_output = ner_output
                break
            except Exception as exc:
                logger.warning("[DiagnosisChain Async] NER 第 %d 次失败: %s", attempt + 1, exc)
                retries += 1
                if attempt == self.max_retries:
                    return DiagnosisResult(
                        success=False,
                        error_message=f"NER 抽取全部重试失败: {exc}",
                        retries_used=retries,
                    )

        # Stage 2: FactCheck
        logger.info("[DiagnosisChain Async] Stage 2/3: FactCheck")
        fact_check_output = None
        for attempt in range(self.max_retries + 1):
            try:
                fact_check_output = await self.fact_check_agent.averify(result.ner_output, markdown_text)
                result.fact_check_output = fact_check_output
                break
            except Exception as exc:
                logger.warning("[DiagnosisChain Async] FactCheck 第 %d 次失败: %s", attempt + 1, exc)
                retries += 1
                if attempt == self.max_retries:
                    logger.warning("[DiagnosisChain Async] FactCheck 降级为全部 correct")
                    break

        # Stage 3: Supplement
        logger.info("[DiagnosisChain Async] Stage 3/3: Supplement")
        supplement_output = None
        for attempt in range(self.max_retries + 1):
            try:
                supplement_output = await self.supplement_agent.adetect_gaps(result.ner_output, markdown_text)
                result.supplement_output = supplement_output
                break
            except Exception as exc:
                logger.warning("[DiagnosisChain Async] Supplement 第 %d 次失败: %s", attempt + 1, exc)
                retries += 1
                if attempt == self.max_retries:
                    logger.warning("[DiagnosisChain Async] Supplement 跳过")
                    break

        result.output = self._aggregate(result.ner_output, fact_check_output, supplement_output)
        result.success = True
        result.retries_used = retries
        return result


# 模块级单例
_diagnosis_chain: DiagnosisChain | None = None


def get_diagnosis_chain() -> DiagnosisChain:
    """获取诊断链单例"""
    global _diagnosis_chain
    if _diagnosis_chain is None:
        _diagnosis_chain = DiagnosisChain()
    return _diagnosis_chain
