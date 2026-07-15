"""
LangChain Agent 模块

基于 LangChain 框架的 AI 智能体模块，独立于现有的 deepseek_service.py。
提供三大核心 Agent：
- NER Agent：实体与关系抽取
- FactCheck Agent：基于 CoT 的事实校验
- Supplement Agent：逻辑断层检测与知识补全

以及：
- DiagnosisChain：三 Agent 编排流水线
- router.py：FastAPI 路由（/api/langchain/*）
"""
from app.langchain_agent.agents import (
    NERAgent,
    FactCheckAgent,
    SupplementAgent,
    get_ner_agent,
    get_fact_check_agent,
    get_supplement_agent,
)
from app.langchain_agent.chains import (
    DiagnosisChain,
    DiagnosisResult,
    get_diagnosis_chain,
)
from app.langchain_agent.router import router as langchain_router

__all__ = [
    "NERAgent",
    "FactCheckAgent",
    "SupplementAgent",
    "DiagnosisChain",
    "DiagnosisResult",
    "get_ner_agent",
    "get_fact_check_agent",
    "get_supplement_agent",
    "get_diagnosis_chain",
    "langchain_router",
]
