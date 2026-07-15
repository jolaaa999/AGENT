"""
图谱工具函数

提供给 LangChain Agent 使用的工具（Tool），
Agent 可在推理过程中调用这些工具来查询知识图谱信息。
"""

import json
import logging
from typing import Optional

logger = logging.getLogger(__name__)


def format_graph_context(
    nodes_json: str = "[]",
    edges_json: str = "[]",
    max_nodes: int = 50,
) -> str:
    """
    将图谱数据格式化为 LLM 可读的上下文文本。

    用于右侧 AI 对话面板：Agent 读取当前文件的图谱数据，
    以此为上下文回答学生的问题。

    Args:
        nodes_json: JSON 格式的节点列表
        edges_json: JSON 格式的边列表
        max_nodes: 最多展示的节点数（防止上下文过长）

    Returns:
        格式化的图描述文本
    """
    try:
        nodes: list[dict] = json.loads(nodes_json) if isinstance(nodes_json, str) else nodes_json
        edges: list[dict] = json.loads(edges_json) if isinstance(edges_json, str) else edges_json
    except (json.JSONDecodeError, TypeError):
        logger.warning("图谱数据 JSON 解析失败，返回空上下文")
        return "（当前无法读取图谱数据）"

    if not nodes:
        return "（当前图谱中暂无节点）"

    # 统计状态分布
    error_nodes = [n for n in nodes if n.get("status") == "error"]
    supplement_nodes = [n for n in nodes if n.get("status") == "supplement"]
    correct_nodes = [n for n in nodes if n.get("status") == "correct"]

    lines = [
        f"## 当前知识图谱概况",
        f"- 总节点数: {len(nodes)}（correct: {len(correct_nodes)}, error: {len(error_nodes)}, supplement: {len(supplement_nodes)}）",
        f"- 总边数: {len(edges)}",
        "",
    ]

    # 优先展示 error 节点（学生最需要关注）
    if error_nodes:
        lines.append("### ⚠️ 错误节点（需要纠正）")
        for n in error_nodes[:5]:
            lines.append(f"- **{n.get('name', '(未知)')}**: {n.get('reason', '')}")
        lines.append("")

    # 其次展示 supplement 节点（知识缺口）
    if supplement_nodes:
        lines.append("### 📝 AI 补全节点（知识缺口）")
        for n in supplement_nodes[:5]:
            lines.append(f"- **{n.get('name', '(未知)')}**: {n.get('reason', '')}")
        lines.append("")

    # 所有节点列表（截断）
    lines.append("### 全部概念节点")
    for n in nodes[:max_nodes]:
        name = n.get("name", "(未知)")
        definition = n.get("definition", "")
        status = n.get("status", "correct")
        status_mark = {
            "correct": "✅",
            "error": "❌",
            "supplement": "📝",
        }.get(status, "")
        def_snippet = definition[:80] + "..." if len(definition) > 80 else definition
        lines.append(f"- {status_mark} **{name}**" + (f": {def_snippet}" if def_snippet else ""))

    if len(nodes) > max_nodes:
        lines.append(f"... 还有 {len(nodes) - max_nodes} 个节点未展示")

    # 关键关系
    if edges:
        lines.append("")
        lines.append("### 关键关系")
        # 按类型分组展示
        prereq_edges = [e for e in edges if "PREREQUISITE" in str(e.get("relation", ""))]
        belongs_edges = [e for e in edges if "BELONGS" in str(e.get("relation", ""))]
        if prereq_edges[:5]:
            lines.append("**前置依赖关系 (PREREQUISITE_OF):**")
            for e in prereq_edges[:5]:
                lines.append(f"- {e.get('source', '?')} -> {e.get('target', '?')}")
        if belongs_edges[:5]:
            lines.append("**从属关系 (BELONGS_TO):**")
            for e in belongs_edges[:5]:
                lines.append(f"- {e.get('source', '?')} -> {e.get('target', '?')}")

    return "\n".join(lines)


def format_dependency_tree(
    concept_name: str,
    dependency_tree: list[dict],
) -> str:
    """
    将 DFS 逆向依赖树格式化为学习路径描述。

    Args:
        concept_name: 目标概念
        dependency_tree: 依赖树数据，每项含 {name, depth, relation}

    Returns:
        格式化的依赖树文本
    """
    if not dependency_tree:
        return f"「{concept_name}」在知识图谱中暂无前置依赖记录。"

    # 按 depth 分组
    by_depth: dict[int, list[dict]] = {}
    for node in dependency_tree:
        depth = node.get("depth", 0)
        by_depth.setdefault(depth, []).append(node)

    lines = [
        f"## 「{concept_name}」的逆向学习技能树",
        f"共 {len(dependency_tree)} 个前置知识节点，分布在 {len(by_depth)} 个依赖层级中：",
        "",
    ]

    for depth in sorted(by_depth.keys(), reverse=True):
        nodes_at_depth = by_depth[depth]
        level_name = "基础层" if depth == 0 else f"第{depth}层依赖"
        lines.append(f"### {level_name}（距离目标概念 {depth + 1} 步）")
        for node in nodes_at_depth:
            lines.append(f"- **{node.get('name', '(未知)')}**")
        lines.append("")

    return "\n".join(lines)
