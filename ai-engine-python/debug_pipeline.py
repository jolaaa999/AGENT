"""
调试脚本：逐步执行 LangChain 诊断流水线，打印每一步的中间结果

用法：
    # 用自定义 Markdown 文本
    python debug_pipeline.py --text "笔记内容..."
    
    # 从文件读取
    python debug_pipeline.py --file 笔记.md
    
    # 使用内置示例
    python debug_pipeline.py
"""

import argparse
import json
import sys
import os

# 确保能找到 app 模块
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))


def print_separator(title: str):
    """打印带标题的分隔线"""
    print()
    print("=" * 70)
    print(f"  {title}")
    print("=" * 70)


def print_json(data, label: str = ""):
    """打印格式化的 JSON"""
    if label:
        print(f"\n--- {label} ---")
    try:
        print(json.dumps(data, ensure_ascii=False, indent=2))
    except (TypeError, ValueError):
        print(data)


def main():
    parser = argparse.ArgumentParser(description="LangChain 诊断流水线调试工具")
    parser.add_argument("--text", type=str, default="", help="Markdown 文本（直接输入）")
    parser.add_argument("--file", type=str, default="", help="Markdown 文件路径")
    args = parser.parse_args()

    # ========== 准备输入数据 ==========
    if args.file:
        with open(args.file, encoding="utf-8") as f:
            markdown_text = f.read()
        source = f"文件：{args.file}"
    elif args.text:
        markdown_text = args.text
        source = "命令行输入"
    else:
        # 内置测试数据：数字电路笔记
        markdown_text = """# 数字逻辑基础

## 半加器
半加器是一种能够对两个一位二进制数进行加法运算的组合逻辑电路。
半加器有两个输入（A和B）和两个输出（和S、进位Cout）。
半加器可以处理低位进位。半加器的输入只有两个，所以它是最基本的加法单元。

## 全加器
全加器是在半加器的基础上增加了进位输入，能够处理低位进位。
全加器有三个输入（A、B、进位输入Cin）和两个输出（和S、进位输出Cout）。

## 时序逻辑电路
时序逻辑电路的输出不仅取决于当前的输入，还取决于电路过去的状态。
这使得时序逻辑电路具有记忆功能。"""
        source = "内置示例"

    print(f"\n[输入来源] {source}")
    print(f"[文本长度] {len(markdown_text)} 字符")
    print(f"\n{'─' * 70}")
    print("原文内容：")
    print(f"{'─' * 70}")
    print(markdown_text)

    # ========== 第一步：Markdown 切分 ==========
    print_separator("第 1 步：Markdown 段落切分 (split_markdown_to_chunks)")
    from app.services.markdown_parser import split_markdown_to_chunks

    chunks = split_markdown_to_chunks(markdown_text)
    print(f"\n切分结果：共 {len(chunks)} 个段落\n")
    for i, chunk in enumerate(chunks):
        print(f"  【段落 {i + 1}】（{len(chunk)} 字符）")
        print(f"  {chunk[:200]}{'...' if len(chunk) > 200 else ''}")
        print()

    # ========== 第二步：NER 实体关系抽取 ==========
    print_separator("第 2 步：NER 实体关系抽取")

    from app.langchain_agent.agents import get_ner_agent

    ner_agent = get_ner_agent()
    try:
        ner_output = ner_agent.extract(chunks)
        print(f"\n[OK] NER 抽取成功")
        print(f"   实体：{len(ner_output.entities)} 个")
        print(f"   关系：{len(ner_output.relations)} 条\n")

        print("[实体列表]：")
        for i, e in enumerate(ner_output.entities):
            print(f"  [{i + 1}] {e.name}")
            print(f"      类型：{e.entity_type}")
            print(f"      定义：{e.definition or '(笔记未定义)'[:100]}")
            print()

        print("[关系列表]：")
        for i, r in enumerate(ner_output.relations):
            print(f"  [{i + 1}] {r.source} --[{r.relation.value}]--> {r.target}")
            print(f"      原文依据：{r.evidence[:100] if r.evidence else '(无)'}")
            print()

    except Exception as e:
        print(f"\n[失败] NER 抽取失败：{e}")
        sys.exit(1)

    # ========== 第三步：事实校验 ==========
    print_separator("第 3 步：事实校验 (FactCheck)")

    from app.langchain_agent.agents import get_fact_check_agent

    fact_check_agent = get_fact_check_agent()
    try:
        fact_check_output = fact_check_agent.verify(ner_output, markdown_text)
        print(f"\n[OK] 事实校验完成")
        print(f"   审查条目：{len(fact_check_output.checks)} 条\n")

        for i, c in enumerate(fact_check_output.checks):
            status_icon = "[OK]" if c.status.value == "correct" else "[ERR]"
            print(f"  [{i + 1}] {status_icon} {c.entity_name} → {c.status.value}")
            print(f"      原始陈述：{c.original_claim[:120]}")
            if c.reason:
                print(f"      理由：{c.reason[:200]}")
            if c.corrected_definition:
                print(f"      纠正定义：{c.corrected_definition[:200]}")
            print()

        if fact_check_output.overall_quality:
            print(f"  总评：{fact_check_output.overall_quality}")

    except Exception as e:
        print(f"\n[警告] 事实校验失败（将降级为全部 correct）：{e}")
        fact_check_output = None

    # ========== 第四步：知识补全 ==========
    print_separator("第 4 步：知识补全 / 逻辑断层检测 (Supplement)")

    from app.langchain_agent.agents import get_supplement_agent

    supplement_agent = get_supplement_agent()
    try:
        supplement_output = supplement_agent.detect_gaps(ner_output, markdown_text)
        print(f"\n[OK] 知识补全完成")
        print(f"   补全节点：{len(supplement_output.supplements)} 个\n")

        for i, s in enumerate(supplement_output.supplements):
            print(f"  [{i + 1}] 新概念：{s.supplement_name}")
            print(f"      关联实体：{s.target_entity}")
            print(f"      定义：{s.supplement_definition[:150]}")
            print(f"      补全理由：{s.reason[:200]}")
            print()

        if supplement_output.analysis:
            print(f"  分析：{supplement_output.analysis}")

    except Exception as e:
        print(f"\n[警告] 知识补全失败（将跳过此阶段）：{e}")
        supplement_output = None

    # ========== 第五步：聚合输出 ==========
    print_separator("第 5 步：聚合输出 (DiagnosisOutput)")

    from app.langchain_agent.chains import DiagnosisChain, DiagnosisResult
    from app.langchain_agent.schemas import NodeStatus

    chain = DiagnosisChain()
    aggregated = chain._aggregate(
        ner_output=ner_output,
        fact_check_output=fact_check_output,
        supplement_output=supplement_output,
    )

    print(f"\n[OK] 聚合完成（即最终入库的数据）")
    print(f"   节点总数：{len(aggregated.nodes)}")
    print(f"   边总数：{len(aggregated.edges)}")
    print(f"   摘要：{aggregated.summary}\n")

    # 按状态分组展示节点
    status_groups = {"correct": [], "error": [], "supplement": []}
    for n in aggregated.nodes:
        st = n.status.value if hasattr(n.status, "value") else str(n.status)
        status_groups.setdefault(st, []).append(n)

    if status_groups["error"]:
        print(f"[错误节点]（{len(status_groups['error'])} 个）：")
        for n in status_groups["error"]:
            print(f"    {n.name}：{n.reason[:120]}")
        print()

    if status_groups["supplement"]:
        print(f"[补全节点]（{len(status_groups['supplement'])} 个）：")
        for n in status_groups["supplement"]:
            print(f"    {n.name}：{n.reason[:120]}")
        print()

    if status_groups["correct"]:
        print(f"[正确节点]（{len(status_groups['correct'])} 个）：")
        for n in status_groups["correct"]:
            print(f"    {n.name}", end="")
            if n.definition:
                print(f" → {n.definition[:80]}", end="")
            print()
        print()

    print("[所有关系]：")
    for i, e in enumerate(aggregated.edges):
        print(f"  [{i + 1}] {e.source} --[{e.relation.value}]--> {e.target}  (状态：{e.status.value})")
    print()

    # ========== 完整 JSON 输出（可直接入库的格式） ==========
    print_separator("最终 JSON（Go 后端入库格式）")

    nodes_data = [
        {
            "name": n.name,
            "definition": n.definition,
            "entity_type": n.entity_type,
            "status": n.status.value if hasattr(n.status, "value") else str(n.status),
            "reason": n.reason,
            "source": n.source,
        }
        for n in aggregated.nodes
    ]
    edges_data = [
        {
            "source": e.source,
            "target": e.target,
            "relation": e.relation.value if hasattr(e.relation, "value") else str(e.relation),
            "status": e.status.value if hasattr(e.status, "value") else str(e.status),
            "reason": e.reason,
        }
        for e in aggregated.edges
    ]

    print(f"\n共 {len(nodes_data)} 个节点，{len(edges_data)} 条边\n")
    print_json({"nodes": nodes_data, "edges": edges_data, "summary": aggregated.summary})
    print()
    print("─" * 70)
    print("调试完成 [OK]")
    print("─" * 70)


if __name__ == "__main__":
    main()
