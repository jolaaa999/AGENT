"""Shared LLM client helpers for DeepSeek-compatible APIs."""

from __future__ import annotations

from typing import Any

from app.core.config import settings

# DeepSeek V4 Flash 等模型默认开启 thinking，三 Agent 流水线易超过网关超时。
THINKING_DISABLED: dict[str, Any] = {"thinking": {"type": "disabled"}}


def build_chat_openai(
    *,
    temperature: float = 0.2,
    max_tokens: int = 4096,
    timeout: float = 120,
    max_retries: int = 2,
):
    """构建 LangChain ChatOpenAI（关闭 thinking 以降低延迟）。"""
    from langchain_openai import ChatOpenAI  # noqa: PLC0415

    return ChatOpenAI(
        model=settings.deepseek_model,
        api_key=settings.deepseek_api_key,
        base_url=settings.deepseek_base_url,
        temperature=temperature,
        max_tokens=max_tokens,
        timeout=timeout,
        max_retries=max_retries,
        extra_body=THINKING_DISABLED,
    )
