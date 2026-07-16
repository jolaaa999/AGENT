"""
ReAct Agent 工具集

提供给 AI 对话 Agent 使用的 LangChain Tool 集合，
让 Agent 具备以下能力：
1. 读取/修改当前 Markdown 文档
2. 搜索知识图谱中的概念
3. 分析图像内容（多模态）
"""

import json
import logging
from typing import Optional

from langchain_core.tools import tool

logger = logging.getLogger(__name__)

# ==================== 文档编辑工具 ====================

# 全局文档存储（每个会话一份）
_doc_store: dict[str, dict] = {}


def set_document(conversation_id: str, markdown: str, graph_nodes: list[dict], graph_edges: list[dict]) -> None:
    """注册当前会话的文档和图谱数据"""
    _doc_store[conversation_id] = {
        "markdown": markdown,
        "graph_nodes": graph_nodes,
        "graph_edges": graph_edges,
        "edited": False,
    }


def get_document(conversation_id: str) -> Optional[dict]:
    """获取当前会话的文档数据"""
    return _doc_store.get(conversation_id)


def pop_edited_markdown(conversation_id: str) -> Optional[str]:
    """获取被编辑后的 markdown 并清除编辑标记"""
    doc = _doc_store.get(conversation_id)
    if doc and doc.get("edited"):
        doc["edited"] = False
        return doc["markdown"]
    return None


# ==================== LangChain Tools ====================

@tool
def read_current_markdown(conversation_id: str = "") -> str:
    """
    读取当前正在编辑的 Markdown 笔记全文。

    当你需要查看学生的笔记内容以理解上下文、发现错误、或准备修改时，
    必须先调用此工具获取最新文档内容。

    Args:
        conversation_id: 会话ID（系统自动填入，无需手动传递）
    """
    doc = _doc_store.get(conversation_id)
    if not doc:
        return "（当前没有打开的文档。请先上传一份 Markdown 笔记。）"
    md = doc.get("markdown", "")
    if not md.strip():
        return "（当前文档为空。）"
    # 截断过长文档，保留前 8000 字符
    if len(md) > 8000:
        return md[:8000] + f"\n\n... (文档共 {len(md)} 字符，已截断显示前 8000 字符)"
    return md


@tool
def edit_markdown(
    original_text: str,
    replacement_text: str,
    conversation_id: str = "",
) -> str:
    """
    修改当前 Markdown 文档中的指定内容。

    在原始文档中查找 original_text 并替换为 replacement_text。
    修改后文档会自动保存并重新生成知识图谱。

    Args:
        original_text: 需要被替换的原文片段（必须与文档中的内容精确匹配）
        replacement_text: 替换后的新文本
        conversation_id: 会话ID（系统自动填入）

    Returns:
        操作结果描述
    """
    doc = _doc_store.get(conversation_id)
    if not doc:
        return "错误：当前没有打开的文档，无法编辑。"

    md = doc.get("markdown", "")
    if original_text not in md:
        # 尝试模糊匹配：去掉首尾空白
        stripped = original_text.strip()
        if stripped in md:
            original_text = stripped
        else:
            return (
                f"错误：在文档中未找到指定文本。"
                f"请使用 read_current_markdown 工具确认文档内容后再试。"
                f"\n你要替换的文本：'{original_text[:100]}...'"
            )

    doc["markdown"] = md.replace(original_text, replacement_text, 1)
    doc["edited"] = True
    logger.info("文档已编辑，替换了 %d 字符 -> %d 字符", len(original_text), len(replacement_text))
    return (
        f"文档已修改成功！\n"
        f"替换了：'{original_text[:80]}{'...' if len(original_text) > 80 else ''}'\n"
        f"改为：'{replacement_text[:80]}{'...' if len(replacement_text) > 80 else ''}'\n"
        f"系统将自动重新生成知识图谱以反映变更。"
    )


@tool
def search_knowledge_graph(
    query: str,
    conversation_id: str = "",
) -> str:
    """
    在当前知识图谱中搜索相关概念。

    当你需要了解某个概念的前置依赖、关联知识点、或其在图谱中的状态
    （correct/error/supplement）时，调用此工具。

    Args:
        query: 搜索关键词（可以是概念名、术语等）
        conversation_id: 会话ID（系统自动填入）

    Returns:
        匹配到的概念及其关系信息（JSON 格式）
    """
    doc = _doc_store.get(conversation_id)
    if not doc:
        return "（当前没有可用的知识图谱。）"

    nodes: list[dict] = doc.get("graph_nodes", [])
    edges: list[dict] = doc.get("graph_edges", [])

    if not nodes:
        return "（当前图谱为空，请先生成知识图谱。）"

    query_lower = query.strip().lower()
    results: list[dict] = []

    # 搜索匹配的节点
    for n in nodes:
        name = (n.get("name") or "").lower()
        definition = (n.get("definition") or "").lower()
        if query_lower in name or query_lower in definition:
            results.append({
                "name": n.get("name", ""),
                "status": n.get("status", "correct"),
                "reason": n.get("reason", ""),
                "definition": n.get("definition", "")[:200],
            })

    # 搜索相关边
    related_edges: list[dict] = []
    for e in edges:
        src = (e.get("source") or "").lower()
        tgt = (e.get("target") or "").lower()
        rel = (e.get("relation") or "").lower()
        if query_lower in src or query_lower in tgt or query_lower in rel:
            related_edges.append({
                "source": e.get("source", ""),
                "target": e.get("target", ""),
                "relation": e.get("relation", ""),
                "status": e.get("status", ""),
            })

    if not results and not related_edges:
        return f"未找到与 '{query}' 相关的概念。图谱中共有 {len(nodes)} 个节点。"

    output_parts = []
    if results:
        output_parts.append(f"## 匹配概念 ({len(results)} 个)")
        for r in results[:8]:
            status_icon = {"correct": "✅", "error": "❌", "supplement": "📝"}.get(r["status"], "")
            output_parts.append(f"- {status_icon} **{r['name']}** [{r['status']}]")
            if r.get("reason"):
                output_parts.append(f"  批注：{r['reason'][:150]}")
            if r.get("definition"):
                output_parts.append(f"  定义：{r['definition'][:150]}")

    if related_edges:
        output_parts.append(f"\n## 关联关系 ({len(related_edges)} 条)")
        for e in related_edges[:8]:
            output_parts.append(f"- {e['source']} --[{e['relation']}]--> {e['target']}")

    return "\n".join(output_parts)


@tool
def append_markdown_section(
    new_section: str,
    heading: str = "",
    conversation_id: str = "",
) -> str:
    """
    在 Markdown 文档末尾追加新的章节内容。

    当你需要为学生补充缺失的知识点、添加练习题目、或扩展笔记内容时使用。
    新内容会以 Markdown 标题开头并追加到文档末尾。

    Args:
        new_section: 要追加的新章节内容（Markdown 格式）
        heading: 章节标题（可选，如 "补充：全加器原理"）
        conversation_id: 会话ID（系统自动填入）

    Returns:
        操作结果描述
    """
    doc = _doc_store.get(conversation_id)
    if not doc:
        return "错误：当前没有打开的文档。"

    md = doc["markdown"].rstrip()
    new_content = f"\n\n## {heading}\n\n{new_section}" if heading else f"\n\n{new_section}"
    doc["markdown"] = md + new_content
    doc["edited"] = True

    logger.info("文档追加新章节：%s (%d 字符)", heading, len(new_section))
    return f"已追加新章节 '{heading or '(无标题)'}' 到文档末尾（{len(new_section)} 字符）。系统将重新生成知识图谱。"


# ==================== 注册所有工具 ====================

def get_agent_tools() -> list:
    """获取所有 Agent 工具列表"""
    return [
        read_current_markdown,
        edit_markdown,
        append_markdown_section,
        search_knowledge_graph,
    ]
