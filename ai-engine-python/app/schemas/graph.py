from enum import Enum

from pydantic import BaseModel, Field, model_validator


class RelationStatus(str, Enum):
    correct = "correct"
    error = "error"
    supplement = "supplement"


class ParseRequest(BaseModel):
    markdown: str = Field(..., min_length=1, description="待解析的 Markdown 文本")


class GraphEdge(BaseModel):
    source: str = Field(..., min_length=1)
    target: str = Field(..., min_length=1)
    relation: str = Field(..., min_length=1)
    status: RelationStatus
    reason: str = ""

    @model_validator(mode="after")
    def validate_reason(self) -> "GraphEdge":
        if self.status != RelationStatus.correct and not self.reason.strip():
            raise ValueError("reason is required when status is not 'correct'")
        return self


class ParseResponse(BaseModel):
    chunks: list[str]
    relations: list[GraphEdge]
    retries_used: int


class ExplainRequest(BaseModel):
    concept: str = Field(..., min_length=1, description="需要讲解的知识点")
    markdown: str = Field(..., min_length=1, description="原始 Markdown 笔记")


class ExplainResponse(BaseModel):
    concept: str
    explanation: str
