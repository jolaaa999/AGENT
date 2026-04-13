from markdown_it import MarkdownIt
from markdown_it.token import Token


def _collect_inline_text(token: Token) -> str:
    if token.type == "inline" and token.content:
        return token.content.strip()
    return ""


def split_markdown_to_chunks(markdown_text: str) -> list[str]:
    """
    使用 markdown-it-py 做 AST 级别的轻量切分。
    规则：
    1) 标题（heading_open）开启新段。
    2) 段落、列表项、代码块等作为逻辑块累积。
    3) 兜底使用空行切分，保证至少返回一个 chunk。
    """
    md = MarkdownIt()
    tokens = md.parse(markdown_text)

    chunks: list[str] = []
    current_parts: list[str] = []

    for token in tokens:
        if token.type == "heading_open" and current_parts:
            chunks.append("\n".join(current_parts).strip())
            current_parts = []

        inline_text = _collect_inline_text(token)
        if inline_text:
            current_parts.append(inline_text)
            continue

        if token.type in {"fence", "code_block"} and token.content.strip():
            current_parts.append(token.content.strip())

    if current_parts:
        chunks.append("\n".join(current_parts).strip())

    # AST 切分为空时，使用空行兜底
    if not chunks:
        fallback = [part.strip() for part in markdown_text.split("\n\n") if part.strip()]
        chunks.extend(fallback)

    return chunks or [markdown_text.strip()]
