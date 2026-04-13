import logging

from fastapi import APIRouter, HTTPException

from app.schemas.graph import ParseRequest, ParseResponse
from app.services.deepseek_service import DeepSeekParseError, parse_relations_with_retry
from app.services.markdown_parser import split_markdown_to_chunks

router = APIRouter(prefix="/api", tags=["parse"])
logger = logging.getLogger(__name__)


@router.post("/parse", response_model=ParseResponse)
def parse_markdown(payload: ParseRequest) -> ParseResponse:
    chunks = split_markdown_to_chunks(payload.markdown)
    if not any(chunk.strip() for chunk in chunks):
        raise HTTPException(status_code=400, detail="markdown content is empty after preprocessing")

    try:
        relations, retries_used = parse_relations_with_retry(chunks=chunks, max_retries=2)
    except DeepSeekParseError as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc
    except Exception as exc:  # noqa: BLE001 - 对外隐藏内部异常细节
        logger.exception("unexpected parse error")
        raise HTTPException(status_code=500, detail="unexpected parse error") from exc

    return ParseResponse(chunks=chunks, relations=relations, retries_used=retries_used)
