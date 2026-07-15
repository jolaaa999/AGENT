"""LangChain Prompt templates package"""
from app.langchain_agent.prompts.templates import (
    get_chat_prompt,
    get_fact_check_prompt,
    get_learning_path_prompt,
    get_ner_prompt,
    get_supplement_prompt,
)

__all__ = [
    "get_ner_prompt",
    "get_fact_check_prompt",
    "get_supplement_prompt",
    "get_chat_prompt",
    "get_learning_path_prompt",
]
