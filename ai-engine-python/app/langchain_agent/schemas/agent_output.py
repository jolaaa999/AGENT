"""
LangChain Agent 模块的结构化输出 Schema

定义 Agent 输出的 Pydantic 模型，用于 LangChain 的 StructuredOutputParser，
确保 LLM 输出严格符合下游 Go 服务与 Neo4j 的入库要求。
"""

from enum import Enum

from pydantic import BaseModel, Field, model_validator


# ==================== 枚举定义 ====================

class NodeStatus(str, Enum):
    """节点诊断状态（与 app/schemas/graph.py 的 RelationStatus 对齐）"""
    correct = "correct"          # 笔记内容正确，直接采纳
    error = "error"              # 笔记存在事实性错误，需要标红纠错
    supplement = "supplement"    # AI 补全的逻辑断层节点


class RelationType(str, Enum):
    """知识图谱关系类型"""
    PREREQUISITE_OF = "PREREQUISITE_OF"    # 前置依赖：A 是 B 的前置知识
    BELONGS_TO = "BELONGS_TO"              # 从属关系：A 属于 B 的子概念
    RELATED_TO = "RELATED_TO"              # 一般关联
    SUPPLEMENTS = "SUPPLEMENTS"            # AI 补全节点补充了目标节点的知识缺口
    CORRECTS = "CORRECTS"                  # AI 纠错节点纠正了目标节点的错误


# ==================== 实体识别 Agent 输出 ====================

class ExtractedEntity(BaseModel):
    """单个命名实体的结构化描述"""
    name: str = Field(..., min_length=1, description="实体/概念名称")
    definition: str = Field(
        default="",
        description="从笔记中提取的概念定义，若笔记未明确定义则为空"
    )
    entity_type: str = Field(
        default="concept",
        description="实体类型：concept(概念) | theorem(定理) | formula(公式) | algorithm(算法) | tool(工具)"
    )


class ExtractedRelation(BaseModel):
    """两个实体之间的关系"""
    source: str = Field(..., min_length=1, description="源节点名称")
    target: str = Field(..., min_length=1, description="目标节点名称")
    relation: RelationType = Field(..., description="关系类型")
    evidence: str = Field(
        default="",
        description="从原文中支撑该关系的证据文本片段"
    )


class NEROutput(BaseModel):
    """NER Agent 的标准化输出"""
    entities: list[ExtractedEntity] = Field(default_factory=list, description="抽取的实体列表（可能为空）")
    relations: list[ExtractedRelation] = Field(default_factory=list, description="实体间关系列表")


# ==================== 事实校验 Agent 输出 ====================

class FactCheckItem(BaseModel):
    """单条事实校验结果"""
    entity_name: str = Field(..., min_length=1, description="被校验的实体名称")
    original_claim: str = Field(..., description="笔记中的原始陈述")
    status: NodeStatus = Field(..., description="校验结论")
    reason: str = Field(
        default="",
        description="当 status=error 时为纠错理由；当 status=correct 时可为空"
    )
    corrected_definition: str = Field(
        default="",
        description="当 status=error 时给出正确描述；否则为空"
    )

    @model_validator(mode="after")
    def validate_error_has_reason(self) -> "FactCheckItem":
        if self.status == NodeStatus.error and not self.reason.strip():
            raise ValueError("status=error 时 reason 字段不能为空，必须给出纠错理由")
        if self.status == NodeStatus.error and not self.corrected_definition.strip():
            raise ValueError("status=error 时 corrected_definition 字段不能为空，必须给出正确描述")
        return self


class FactCheckOutput(BaseModel):
    """Fact-Check Agent 的标准化输出"""
    checks: list[FactCheckItem] = Field(..., min_length=1, description="逐条校验结果")
    overall_quality: str = Field(
        default="",
        description="对整段笔记质量的简短总评（1-2句）"
    )


# ==================== 知识补全 Agent 输出 ====================

class SupplementItem(BaseModel):
    """单条知识补全"""
    target_entity: str = Field(
        ..., min_length=1,
        description="补全节点所关联的已有实体名称（即这个补全是针对哪个概念的）"
    )
    supplement_name: str = Field(
        ..., min_length=1,
        description="补全的新概念名称（笔记中缺失但逻辑上必须存在的前置知识）"
    )
    supplement_definition: str = Field(
        ..., min_length=1,
        description="补全概念的简明定义"
    )
    reason: str = Field(
        ..., min_length=1,
        description="补全理由：说明为什么基于笔记的逻辑链，这个缺失概念是必需的"
    )
    relation_to_target: RelationType = Field(
        default=RelationType.SUPPLEMENTS,
        description="补全节点与目标实体的关系，通常为 SUPPLEMENTS"
    )


class SupplementOutput(BaseModel):
    """Supplement Agent 的标准化输出"""
    supplements: list[SupplementItem] = Field(
        default_factory=list,
        description="补全项列表，若逻辑完整无缺失则为空列表"
    )
    analysis: str = Field(
        default="",
        description="逻辑完整性分析的简要说明"
    )


# ==================== 综合诊断输出（聚合三个 Agent） ====================

class DiagnosisNode(BaseModel):
    """最终入库的图谱节点"""
    name: str = Field(..., min_length=1)
    definition: str = Field(default="")
    entity_type: str = Field(default="concept")
    status: NodeStatus = Field(default=NodeStatus.correct)
    reason: str = Field(default="")
    source: str = Field(default="ner")  # 来源Agent：ner / fact_check / supplement


class DiagnosisEdge(BaseModel):
    """最终入库的图谱边"""
    source: str = Field(..., min_length=1)
    target: str = Field(..., min_length=1)
    relation: RelationType = Field(default=RelationType.RELATED_TO)
    status: NodeStatus = Field(default=NodeStatus.correct)
    reason: str = Field(default="")
    source_agent: str = Field(default="ner")  # 来源Agent


class DiagnosisOutput(BaseModel):
    """LangChain 诊断链的最终聚合输出

    这个 Schema 是三个 Agent 输出合并后的结果，
    直接对应 Go 后端的入库 JSON 格式。
    若所有 Agent 均未能抽取到有效数据，nodes/edges 可为空列表。
    """
    nodes: list[DiagnosisNode] = Field(default_factory=list)
    edges: list[DiagnosisEdge] = Field(default_factory=list)
    summary: str = Field(default="", description="本次诊断的汇总说明")
