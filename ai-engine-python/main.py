from fastapi import FastAPI

from app.api.parse import router as parse_router
from app.langchain_agent import langchain_router
from app.core.config import settings

app = FastAPI(
    title="AI Engine Python",
    description="基于 Markdown AST + LangChain Agent + DeepSeek 的图谱关系解析引擎",
    version="0.3.0",
)
app.include_router(parse_router)
app.include_router(langchain_router)


@app.on_event("startup")
def validate_config_on_startup() -> None:
    settings.validate_or_raise()


@app.get("/health")
def health() -> dict[str, str]:
    return {"service": "ai-engine-python", "status": "ok"}


@app.get("/health/deepseek")
def deepseek_config_health() -> dict[str, str]:
    return {
        "service": "deepseek-api-config",
        "status": "configured" if settings.deepseek_api_key else "missing_api_key",
    }
