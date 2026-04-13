import json
import logging
from typing import Any

from openai import OpenAI
from pydantic import ValidationError

from app.core.config import settings
from app.schemas.graph import GraphEdge

SYSTEM_PROMPT = """
你是“严厉的学术审稿人”。
任务：从给定文本中抽取知识图谱关系，并对关系进行严格审查。

你必须仅输出 JSON 数组，不允许任何额外文本，不允许 Markdown 代码块。
数组每个元素必须包含以下字段：
- source: 源节点实体名称（字符串）
- target: 目标节点实体名称（字符串）
- relation: 关系描述（字符串）
- status: 只能是 "correct" | "error" | "supplement"
- reason: 当 status 为 correct 时可为空字符串；当 status 为 error 或 supplement 时，必须是详细解释

审查标准：
1) correct：文本明确支持且逻辑自洽。
2) error：文本中存在事实性错误、定义混淆或逻辑冲突。
3) supplement：文本存在逻辑断层，需要基于通识先验补全缺失关系。
""".strip()

EXPLAIN_SYSTEM_PROMPT = """
你是一名严谨的跨学科导师。请针对用户给出的知识点，结合原始笔记内容输出“可学习、可执行”的讲解。

输出要求（纯文本）：
1) 先给出该知识点的简明定义（2-3 句）
2) 解释它与笔记中上下文的关系（指出关键前置知识）
3) 给出一个直观例子或类比
4) 给出 3 条下一步学习建议（可执行）

风格要求：准确、结构清晰、避免空泛套话、不要输出 Markdown 代码块。
""".strip()


class DeepSeekParseError(Exception):
    """Raised when LLM output cannot be converted to valid graph edges."""


logger = logging.getLogger(__name__)


def _extract_json_array(raw_text: str) -> list[dict[str, Any]]:
    text = raw_text.strip()

    # 优先直接解析完整响应
    try:
        data = json.loads(text)
        if isinstance(data, list):
            return data
    except json.JSONDecodeError:
        pass

    # 兜底：提取首个 JSON 数组片段
    left = text.find("[")
    right = text.rfind("]")
    if left != -1 and right != -1 and left < right:
        candidate = text[left : right + 1]
        data = json.loads(candidate)
        if isinstance(data, list):
            return data

    raise DeepSeekParseError("model output is not a valid JSON array")


def _validate_edges(items: list[dict[str, Any]]) -> list[GraphEdge]:
    try:
        return [GraphEdge.model_validate(item) for item in items]
    except ValidationError as exc:
        raise DeepSeekParseError(f"json schema validation failed: {exc}") from exc


def _build_user_prompt(chunks: list[str]) -> str:
    chunk_text = "\n\n".join(
        [f"[Chunk {index + 1}]\n{chunk}" for index, chunk in enumerate(chunks)]
    )
    return f"请审稿并抽取关系，输入内容如下：\n\n{chunk_text}"


def parse_relations_with_retry(chunks: list[str], max_retries: int = 2) -> tuple[list[GraphEdge], int]:
    if not settings.deepseek_api_key.strip():
        raise DeepSeekParseError("DEEPSEEK_API_KEY is missing")

    client = OpenAI(
        api_key=settings.deepseek_api_key,
        base_url=settings.deepseek_base_url,
    )

    last_error: Exception | None = None
    total_attempts = max_retries + 1
    user_prompt = _build_user_prompt(chunks)

    for attempt in range(total_attempts):
        try:
            completion = client.chat.completions.create(
                model=settings.deepseek_model,
                messages=[
                    {"role": "system", "content": SYSTEM_PROMPT},
                    {"role": "user", "content": user_prompt},
                ],
                temperature=0.2,
            )
            content = completion.choices[0].message.content or ""
            if not content.strip():
                raise DeepSeekParseError("deepseek returned empty content")
        except Exception as exc:  # noqa: BLE001 - SDK/network/provider errors need retries
            logger.warning("deepseek request failed on attempt %s/%s: %s", attempt + 1, total_attempts, exc)
            last_error = DeepSeekParseError(f"deepseek request failed: {exc}")
            continue

        try:
            raw_items = _extract_json_array(content)
            edges = _validate_edges(raw_items)
            return edges, attempt
        except Exception as exc:  # noqa: BLE001 - 需要兜底重试
            logger.warning("deepseek parse failed on attempt %s/%s: %s", attempt + 1, total_attempts, exc)
            last_error = exc
            continue

    raise DeepSeekParseError(f"failed to parse model output after retries: {last_error}")


def explain_concept(concept: str, markdown: str) -> str:
    if not settings.deepseek_api_key.strip():
        raise DeepSeekParseError("DEEPSEEK_API_KEY is missing")

    client = OpenAI(
        api_key=settings.deepseek_api_key,
        base_url=settings.deepseek_base_url,
    )
    user_prompt = f"目标知识点：{concept}\n\n原始笔记：\n{markdown}"

    try:
        completion = client.chat.completions.create(
            model=settings.deepseek_model,
            messages=[
                {"role": "system", "content": EXPLAIN_SYSTEM_PROMPT},
                {"role": "user", "content": user_prompt},
            ],
            temperature=0.3,
        )
    except Exception as exc:  # noqa: BLE001 - provider/network errors
        raise DeepSeekParseError(f"deepseek explain request failed: {exc}") from exc

    content = (completion.choices[0].message.content or "").strip()
    if not content:
        raise DeepSeekParseError("deepseek explain returned empty content")
    return content
