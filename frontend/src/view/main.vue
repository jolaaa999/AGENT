<script setup lang="ts">
import { Graph } from "@antv/g6";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { explainConcept, getGraphAll, getGraphPath, getNodeNeighbors, uploadNote, type GraphResponse } from "../api/graph";
import {
  buildFocusSet,
  buildStyledGraph,
  preprocessGraphData,
  getLayoutConfig,
  getNodeConfig,
  getEdgeConfig,
  getCollideLayoutConfig,
  type LayoutType
} from "../graph/g6-config";

const markdown = ref("");
const concept = ref("");
const userId = ref("");
const maxDepth = ref(3);
const isLoading = ref(false);
const isNavigating = ref(false);
const statusText = ref("图谱待生成");
const importedFileName = ref("");
const graphRoot = ref<HTMLDivElement | null>(null);
  // 绑定图谱容器，引用dom元素，可以获取容器大小高度↑
const selectedNodeDetail = ref<{
  id: string;
  label: string;
  type?: string;
  status?: string;
  reason?: string;
  snippets: string[];
  aiExplanation: string;
} | null>(null);
const isExplaining = ref(false);

// LightRAG 渐进式展开模式状态 - 默认开启
const isLightRAGMode = ref(true);
const expandedNodes = ref<Set<string>>(new Set());

let graph: Graph | null = null;
let graphRawData: GraphResponse = { nodes: [], edges: [] };
let resizeObserver: ResizeObserver | null = null;

const canGenerate = computed(() => markdown.value.trim().length > 0);
const canNavigate = computed(() => concept.value.trim().length > 0);

function tooltipHtml(reason: string) {
  return `
    <div style="
      max-width: 260px;
      border: 1px solid #E2E8F0;
      border-radius: 12px;
      background: rgba(15, 23, 42, 0.95);
      color: #F8FAFC;
      padding: 10px 12px;
      box-shadow: 0 10px 30px rgba(15, 23, 42, 0.25);
      font-size: 12px;
      line-height: 1.5;
      letter-spacing: 0.01em;
    ">
      ${reason || "暂无批注原因"}
    </div>
  `;
}

function extractMarkdownSnippets(concept: string, content: string): string[] {
  if (!concept.trim() || !content.trim()) return [];
  const lines = content.split(/\r?\n/);
  const normalizedConcept = concept.toLowerCase();
  const snippets: string[] = [];
  const usedRanges = new Set<string>();

  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i];
    if (!line.toLowerCase().includes(normalizedConcept)) continue;

    const start = Math.max(0, i - 1);
    const end = Math.min(lines.length - 1, i + 1);
    const rangeKey = `${start}-${end}`;
    if (usedRanges.has(rangeKey)) continue;
    usedRanges.add(rangeKey);

    const snippet = lines
      .slice(start, end + 1)
      .join("\n")
      .trim();
    if (snippet) snippets.push(snippet);
    if (snippets.length >= 3) break;
  }

  return snippets;
}

function buildAIExplanation(node: { label: string; status?: string; reason?: string }, snippets: string[]): string {
  const statusText =
    node.status === "correct"
      ? "该知识点在当前笔记中逻辑基本成立。"
      : node.status === "error"
        ? "该知识点存在明显错误或概念混淆，需要优先纠正。"
        : node.status === "supplement"
          ? "该知识点存在逻辑断层，建议补全关键前置知识。"
          : "该知识点已被识别，但状态信息不足。";

  const snippetHint = snippets.length
    ? `基于原笔记可提取到 ${snippets.length} 处相关描述，建议先通读这些原文，再进行深度学习。`
    : "原笔记中未检索到明显原文描述，建议补充定义、公式和例子后再分析。";

  const reason = node.reason?.trim()
    ? `系统批注指出：${node.reason.trim()}`
    : "当前未提供批注原因，可结合上下游关联节点进行补充判断。";

  return `${statusText}\n\n${reason}\n\n${snippetHint}\n\n建议学习动作：\n1. 先写出该概念的一句话定义。\n2. 列出至少 2 个前置知识点与 1 个应用场景。\n3. 用自己的话复述并回填到 Markdown 笔记。`;
}

function resolveNodeIdFromEvent(event: any): string {
  const candidates = [
    event?.data?.id,
    event?.data?.data?.id,
    event?.target?.id,
    event?.target?.config?.id,
    event?.target?.data?.id,
    event?.itemId
  ];
  const hit = candidates.find((value) => typeof value === "string" && value.trim().length > 0);
  return hit ? String(hit) : "";
}

async function showNodeDetail(nodeId: string) {
  const node = graphRawData.nodes.find((item) => item.id === nodeId);
  if (!node) return;
  const snippets = extractMarkdownSnippets(node.label || node.id, markdown.value);

  selectedNodeDetail.value = {
    id: node.id,
    label: node.label || node.id,
    type: node.type || String(node.data?.type ?? ""),
    status: node.status || String(node.data?.status ?? ""),
    reason: node.reason || String(node.data?.reason ?? ""),
    snippets,
    aiExplanation: buildAIExplanation(
      {
        label: node.label || node.id,
        status: node.status || String(node.data?.status ?? ""),
        reason: node.reason || String(node.data?.reason ?? "")
      },
      snippets
    )
  };

  if (!markdown.value.trim()) {
    selectedNodeDetail.value.aiExplanation = "请先输入或导入 Markdown 笔记，再生成该知识点的专业讲解。";
    return;
  }

  isExplaining.value = true;
  try {
    const result = await explainConcept({
      concept: node.label || node.id,
      markdown: markdown.value,
      user_id: userId.value.trim() || undefined
    });
    if (selectedNodeDetail.value?.id === node.id) {
      selectedNodeDetail.value.aiExplanation = result.explanation;
    }
  } catch (error) {
    if (selectedNodeDetail.value?.id === node.id) {
      selectedNodeDetail.value.aiExplanation = `讲解生成失败：${(error as Error).message}`;
    }
  } finally {
    isExplaining.value = false;
  }
}

/**
 * LightRAG 模式：扇形径向展开节点邻居
 * 点击核心节点时，邻居沿远离核心的方向均匀分布（圆周排列）
 * 核心节点位置保持不变
 */
async function expandNodeNeighbors(nodeId: string) {
  if (!graph) return;

  statusText.value = `正在展开节点 "${nodeId}" 的邻居...`;
  try {
    const neighbors = await getNodeNeighbors(nodeId, userId.value.trim() || undefined, 1);

    // 过滤出尚未显示的节点和边
    const existingNodeIds = new Set(graphRawData.nodes.map((n) => n.id));
    const existingEdgeIds = new Set(graphRawData.edges.map((e) => e.id));

    const newNodes = neighbors.nodes.filter((n) => !existingNodeIds.has(n.id));
    const newEdges = neighbors.edges.filter((e) => !existingEdgeIds.has(e.id));

    if (newNodes.length === 0 && newEdges.length === 0) {
      statusText.value = `节点 "${nodeId}" 没有更多邻居`;
      expandedNodes.value.add(nodeId);
      return;
    }

    // 获取核心节点的当前位置
    const graphData = graph.getData();
    const centerNode = graphData.nodes?.find((n: any) => n.id === nodeId);
    if (!centerNode || centerNode.x === undefined || centerNode.y === undefined) {
      console.error("[展开] 无法获取核心节点位置");
      return;
    }

    const centerX = centerNode.x as number;
    const centerY = centerNode.y as number;
    const radius = 180; // 邻居节点分布的半径

    // 为每个新节点计算扇形径向位置
    const angleStep = (2 * Math.PI) / newNodes.length;
    const positionedNodes = newNodes.map((node, index) => {
      const angle = index * angleStep - Math.PI / 2; // 从顶部开始顺时针排列
      return {
        ...node,
        x: centerX + radius * Math.cos(angle),
        y: centerY + radius * Math.sin(angle)
      };
    });

    // 更新本地数据
    graphRawData.nodes.push(...positionedNodes);
    graphRawData.edges.push(...newEdges);

    // 标记节点已展开
    expandedNodes.value.add(nodeId);

    // 增量添加到图谱（带预计算位置）
    const styled = buildStyledGraph({ nodes: positionedNodes, edges: newEdges });
    graph.addData(styled as any);

    // 使用 radial 布局或保持当前位置
    await graph.render();

    statusText.value = `已展开 ${newNodes.length} 个节点，${newEdges.length} 条边`;
  } catch (error) {
    statusText.value = `展开邻居失败：${(error as Error).message}`;
  }
}

async function initGraph() {
  if (!graphRoot.value) return;

  const { width, height } = graphRoot.value.getBoundingClientRect();
  graph = new Graph({
    container: graphRoot.value,
    width,
    height,
    autoFit: "view",
    data: { nodes: [], edges: [] },
    node: getNodeConfig(),
    edge: getEdgeConfig(),
    layout: getLayoutConfig("force"),
    behaviors: ["drag-canvas", "zoom-canvas", "drag-element"],
    //有节点报错或者supplement的时候，用户鼠标悬停显示提示框以及原因
    plugins: [
      {
        type: "tooltip",
        trigger: "hover",
        enable: (event: { targetType?: string; target?: { data?: Record<string, unknown> } }) => {
          if (event.targetType !== "node") return false;
          const status = String(event.target?.data?.status ?? "");
          return status === "error" || status === "supplement";
        },
        getContent: (event: { target?: { data?: Record<string, unknown> } }) =>
          tooltipHtml(String(event.target?.data?.reason ?? ""))
      }
    ]
     //有节点报错或者supplement的时候，用户鼠标悬停显示提示框以及原因
  });
  // 实际渲染函数
  await graph.render();
  //graph.on 是 G6 图可视化引擎中的事件监听方法
 // ↓ 点击节点显示节点详情或展开邻居（LightRAG 模式）
  graph.on("node:click", (event: any) => {
    const nodeId = resolveNodeIdFromEvent(event);
    if (!nodeId) return;

    // LightRAG 模式：点击节点展开邻居
    if (isLightRAGMode.value && !expandedNodes.value.has(nodeId)) {
      void expandNodeNeighbors(nodeId);
    } else {
      void showNodeDetail(nodeId);
    }
  });
 // ↓ 显示节点详情，G6 不同版本的事件系统可能有差异，node:click 和 element:click 是两种事件格式，双重绑定确保兼容性。
  graph.on("element:click", (event: any) => {
    if (event?.targetType !== "node") return;
    const nodeId = resolveNodeIdFromEvent(event);
    if (!nodeId) return;

    // LightRAG 模式：点击节点展开邻居
    if (isLightRAGMode.value && !expandedNodes.value.has(nodeId)) {
      void expandNodeNeighbors(nodeId);
    } else {
      void showNodeDetail(nodeId);
    }
  });

  // ↓ 点击画布空白区域时，关闭节点详情面板
  graph.on("canvas:click", () => {
    selectedNodeDetail.value = null;
  });
}

async function renderGraph(
  data: GraphResponse,
  focusMode = false,
  pathData: GraphResponse[] = [],
  layoutType: LayoutType = "force"
) {
  if (!graph) return;

  console.log("[渲染] 开始渲染，节点数:", data.nodes.length, "边数:", data.edges.length);

  const focusSet = focusMode ? buildFocusSet(pathData) : undefined;
  const styled = buildStyledGraph(data, focusSet);

  // 检查 styled 数据中是否有重复 ID
  const styledIds = styled.nodes.map((n: any) => n.id);
  const uniqueIds = new Set(styledIds);
  if (styledIds.length !== uniqueIds.size) {
    console.error("[渲染] 错误: styled 数据中有重复 ID");
    const counts = new Map<string, number>();
    styledIds.forEach((id: string) => counts.set(id, (counts.get(id) || 0) + 1));
    const dups = Array.from(counts.entries()).filter(([_, c]) => c > 1);
    console.error("[渲染] 重复 ID:", dups);
  }

  // 检查是否有节点带有预计算位置（扇形展开）
  const hasPresetPositions = styled.nodes.some((n: any) => n.x !== undefined && n.y !== undefined);

  if (hasPresetPositions) {
    // 如果有预计算位置，使用 preset 布局保持位置
    console.log("[渲染] 使用 preset 布局（保留预计算位置）");
    graph.setLayout({
      type: "preset",
      padding: 50
    });
  } else {
    // 根据模式切换布局
    const layoutConfig = getLayoutConfig(layoutType);
    console.log("[渲染] 使用布局:", layoutType);
    graph.setLayout(layoutConfig);
  }

  graph.setData(styled as any);
  await graph.render();

  // 渲染后检查节点位置
  const graphData = graph.getData();
  console.log("[渲染] 渲染完成，实际节点数:", graphData.nodes?.length);

  // 检查是否有节点位置重叠
  if (graphData.nodes && graphData.nodes.length > 0) {
    const positions = graphData.nodes.map((n: any) => ({ id: n.id, x: n.x, y: n.y }));
    console.log("[渲染] 所有节点位置:", positions);

    // 检查距离过近的节点对
    const overlaps: Array<{ id1: string; id2: string; distance: number }> = [];
    for (let i = 0; i < positions.length; i++) {
      for (let j = i + 1; j < positions.length; j++) {
        const p1 = positions[i];
        const p2 = positions[j];
        if (p1.x !== undefined && p1.y !== undefined && p2.x !== undefined && p2.y !== undefined) {
          const dist = Math.sqrt(Math.pow(p1.x - p2.x, 2) + Math.pow(p1.y - p2.y, 2));
          if (dist < 90) { // 节点直径约 70+padding，距离小于 90 算重叠
            overlaps.push({ id1: p1.id, id2: p2.id, distance: dist });
          }
        } else {
          console.warn("[渲染] 节点位置未定义:", p1.id, p1.x, p1.y, p2.id, p2.x, p2.y);
        }
      }
    }
    if (overlaps.length > 0) {
      console.warn("[渲染] 发现距离过近的节点对 (可能重叠):", overlaps);
    } else {
      console.log("[渲染] 未发现重叠节点");
    }
  }
}

async function fetchAllGraph() {
  console.log("[fetchAllGraph] 开始获取图谱数据...");
  const rawData = await getGraphAll(userId.value.trim() || undefined);
  console.log("[fetchAllGraph] 原始数据:", { nodes: rawData.nodes.length, edges: rawData.edges.length });

  // 应用数据预处理：合并同义节点、过滤低质量边、降低噪声
  graphRawData = preprocessGraphData(rawData.nodes, rawData.edges, {
    minConfidence: 0.6,
    removeSelfLoops: true,
    keepIsolatedNodes: false
  });
  console.log("[fetchAllGraph] 预处理后:", { nodes: graphRawData.nodes.length, edges: graphRawData.edges.length });

  // LightRAG 模式：只显示核心节点（连接数最多的前 N 个）
  if (isLightRAGMode.value) {
    console.log("[fetchAllGraph] LightRAG 模式：提取核心节点");
    const coreNodes = extractCoreNodes(graphRawData, 5); // 默认显示 5 个核心节点
    console.log("[fetchAllGraph] 核心节点:", { nodes: coreNodes.nodes.length, edges: coreNodes.edges.length });
    console.log("[fetchAllGraph] 核心节点 IDs:", coreNodes.nodes.map((n) => n.id));
    graphRawData = coreNodes;
    expandedNodes.value.clear();
  }

  await renderGraph(graphRawData);
  statusText.value = `图谱已加载：${graphRawData.nodes.length} 节点 / ${graphRawData.edges.length} 连线${isLightRAGMode.value ? " (渐进式展开模式)" : ""}`;
}

/**
 * 提取核心节点（连接数最多的前 N 个）
 * 用于 LightRAG 模式的初始展示
 */
function extractCoreNodes(data: GraphResponse, count: number): GraphResponse {
  // 计算每个节点的连接数
  const nodeDegree = new Map<string, number>();
  data.nodes.forEach((node) => nodeDegree.set(node.id, 0));

  data.edges.forEach((edge) => {
    nodeDegree.set(edge.source, (nodeDegree.get(edge.source) || 0) + 1);
    nodeDegree.set(edge.target, (nodeDegree.get(edge.target) || 0) + 1);
  });

  // 打印节点度数排序
  const degreeEntries = Array.from(nodeDegree.entries())
    .sort((a, b) => b[1] - a[1])
    .slice(0, 10);
  console.log("[extractCoreNodes] 节点度数 TOP 10:", degreeEntries);

  // 按连接数排序，取前 N 个
  const sortedNodes = [...data.nodes]
    .sort((a, b) => (nodeDegree.get(b.id) || 0) - (nodeDegree.get(a.id) || 0))
    .slice(0, count);

  const coreNodeIds = new Set(sortedNodes.map((n) => n.id));

  // 过滤出核心节点之间的边
  const coreEdges = data.edges.filter(
    (e) => coreNodeIds.has(e.source) && coreNodeIds.has(e.target)
  );

  console.log("[extractCoreNodes] 核心节点间边数:", coreEdges.length);

  return {
    nodes: sortedNodes,
    edges: coreEdges
  };
}

async function handleUpload() {
  if (!canGenerate.value) return;
  isLoading.value = true;
  statusText.value = "正在解析 Markdown 并同步图谱...";
  try {
    await uploadNote({
      markdown: markdown.value,
      user_id: userId.value.trim() || undefined
    });
    await fetchAllGraph();
    isNavigating.value = false;
    selectedNodeDetail.value = null;
  } catch (error) {
    statusText.value = `生成失败：${(error as Error).message}`;
  } finally {
    isLoading.value = false;
  }
}

async function handleImportMarkdownFile(event: Event) {
  const input = event.target as HTMLInputElement | null;
  const file = input?.files?.[0];
  if (!file) return;

  const isMarkdown = file.name.toLowerCase().endsWith(".md") || file.type === "text/markdown";
  if (!isMarkdown) {
    statusText.value = "仅支持导入 .md Markdown 文件";
    input.value = "";
    return;
  }

  const maxBytes = 2 * 1024 * 1024;
  if (file.size > maxBytes) {
    statusText.value = "Markdown 文件过大，请控制在 2MB 以内";
    input.value = "";
    return;
  }

  try {
    markdown.value = await file.text();
    importedFileName.value = file.name;
    statusText.value = `已导入文件：${file.name}`;
  } catch (error) {
    statusText.value = `读取文件失败：${(error as Error).message}`;
  } finally {
    input.value = "";
  }
}

async function handlePathNavigate() {
  if (!canNavigate.value) return;
  statusText.value = "正在计算逆向学习路径...";
  try {
    const result = await getGraphPath(concept.value.trim(), userId.value.trim() || undefined, maxDepth.value);
    // 逆向导航使用 Dagre 层级布局，更适合展示学习路径的依赖关系
    await renderGraph(graphRawData, true, result.paths, "dagre");
    isNavigating.value = true;
    statusText.value = `专注模式已开启：命中 ${result.paths.length} 条依赖路径`;
  } catch (error) {
    statusText.value = `路径查询失败：${(error as Error).message}`;
  }
}

async function resetFocus() {
  // 退出专注模式，恢复力导向布局
  await renderGraph(graphRawData, false, [], "force");
  isNavigating.value = false;
  statusText.value = "已退出专注模式";
}

onMounted(async () => {
  await initGraph();
  await fetchAllGraph();

  if (graphRoot.value && graph) {
    resizeObserver = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (!entry || !graph) return;
      const { width, height } = entry.contentRect;
      graph.resize(width, height);
    });
    resizeObserver.observe(graphRoot.value);
  }
});

onBeforeUnmount(() => {
  resizeObserver?.disconnect();
  graph?.destroy();
  graph = null;
});
</script>

<template>
  <main class="relative min-h-screen bg-slate-100/80 p-5 md:p-8">
    <div class="grid w-full grid-cols-1 gap-5 xl:grid-cols-[1fr_2fr]">
      <section class="rounded-2xl border border-slate-200/80 bg-white/95 p-6 shadow-[0_10px_40px_rgba(15,23,42,0.05)] xl:h-[calc(100vh-96px)] xl:overflow-y-auto">
        <header class="mb-5">
          <h1 class="text-[22px] font-semibold tracking-tight text-slate-900">Knowledge Studio</h1>
          <p class="mt-2 text-sm leading-relaxed text-slate-500">
            粘贴 Markdown，生成可解释的知识图谱，并对目标概念执行逆向导航学习。
          </p>
        </header>

        <label class="mb-2 block text-xs font-medium uppercase tracking-[0.12em] text-slate-400">
          Markdown 笔记输入
        </label>
        <div class="mb-3 flex items-center justify-between gap-3 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2.5">
          <label class="inline-flex cursor-pointer items-center rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-xs font-medium text-slate-700 transition hover:bg-slate-50">
            导入 .md 文件
            <input class="hidden" type="file" accept=".md,text/markdown" @change="handleImportMarkdownFile" />
          </label>
          <span class="truncate text-xs text-slate-500">
            {{ importedFileName || "未选择文件" }}
          </span>
        </div>
        <textarea
          v-model="markdown"
          class="h-72 w-full resize-none rounded-xl border border-slate-200 bg-slate-50/70 p-4 text-sm leading-relaxed text-slate-700 outline-none transition focus:border-blue-400 focus:bg-white xl:h-[min(44vh,420px)]"
          placeholder="在这里粘贴你的课程笔记、读书摘要或错题复盘..."
        />

        <div class="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-[1fr_auto]">
          <input
            v-model="userId"
            class="rounded-xl border border-slate-200 bg-white px-3 py-2.5 text-sm text-slate-700 outline-none focus:border-slate-400"
            placeholder="可选：user_id（用于多用户隔离）"
          />
          <button
            :disabled="!canGenerate || isLoading"
            class="rounded-xl bg-slate-900 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-50"
            @click="handleUpload"
          >
            一键生成/体检知识图谱
          </button>
        </div>

        <div class="mt-6 border-t border-slate-200 pt-5">
          <label class="mb-2 block text-xs font-medium uppercase tracking-[0.12em] text-slate-400">
            逆向学习导航
          </label>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_110px]">
            <input
              v-model="concept"
              class="rounded-xl border border-slate-200 bg-white px-3 py-2.5 text-sm text-slate-700 outline-none focus:border-violet-400"
              placeholder="输入目标概念，如：机器学习"
            />
            <input
              v-model.number="maxDepth"
              type="number"
              min="1"
              max="6"
              class="rounded-xl border border-slate-200 bg-white px-3 py-2.5 text-sm text-slate-700 outline-none focus:border-violet-400"
              placeholder="深度"
            />
          </div>
          <div class="mt-3 flex gap-3">
            <button
              :disabled="!canNavigate || isLoading"
              class="rounded-xl bg-violet-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-violet-500 disabled:cursor-not-allowed disabled:opacity-50"
              @click="handlePathNavigate"
            >
              逆向学习导航
            </button>
            <button
              v-if="isNavigating"
              class="rounded-xl border border-slate-300 bg-white px-4 py-2.5 text-sm font-medium text-slate-600 transition hover:bg-slate-50"
              @click="resetFocus"
            >
              退出专注模式
            </button>
          </div>
        </div>

        <div class="mt-6 border-t border-slate-200 pt-5">
          <label class="mb-2 block text-xs font-medium uppercase tracking-[0.12em] text-slate-400">
            视图模式
          </label>
          <div class="flex items-center gap-3">
            <button
              :class="[
                'rounded-xl px-4 py-2.5 text-sm font-medium transition',
                isLightRAGMode
                  ? 'bg-emerald-600 text-white hover:bg-emerald-500'
                  : 'border border-slate-300 bg-white text-slate-600 hover:bg-slate-50'
              ]"
              @click="isLightRAGMode = !isLightRAGMode; fetchAllGraph()"
            >
              {{ isLightRAGMode ? '渐进式展开：开' : '渐进式展开：关' }}
            </button>
            <span class="text-xs text-slate-500">
              {{ isLightRAGMode ? '点击节点展开邻居' : '显示完整图谱' }}
            </span>
          </div>
        </div>
      </section>

      <section class="relative overflow-hidden rounded-2xl border border-slate-200/80 bg-white shadow-[0_10px_40px_rgba(15,23,42,0.05)] xl:h-[calc(100vh-96px)]">
        <div class="flex items-center justify-between border-b border-slate-200 px-5 py-3.5">
          <p class="text-sm font-medium text-slate-700">图谱可视化区</p>
          <p class="text-xs text-slate-500">{{ statusText }}</p>
        </div>
        <div
          ref="graphRoot"
          class="h-[65vh] min-h-[460px] w-full bg-[radial-gradient(circle_at_20%_20%,#f8fafc,transparent_45%),radial-gradient(circle_at_80%_90%,#eef2ff,transparent_40%)] xl:h-[calc(100%-52px)]"
        />
        <aside
          v-if="selectedNodeDetail"
          class="absolute right-4 top-16 z-20 w-[340px] max-w-[calc(100%-2rem)] rounded-xl border border-slate-200 bg-white/95 p-4 shadow-xl backdrop-blur"
        >
          <div class="mb-2 flex items-start justify-between gap-3">
            <h3 class="text-base font-semibold text-slate-900">{{ selectedNodeDetail.label }}</h3>
            <button
              class="rounded-md px-2 py-1 text-xs text-slate-500 transition hover:bg-slate-100 hover:text-slate-700"
              @click="selectedNodeDetail = null"
            >
              关闭
            </button>
          </div>
          <div class="mb-3 flex flex-wrap gap-2 text-xs">
            <span class="rounded-md bg-slate-100 px-2 py-1 text-slate-600">
              类型：{{ selectedNodeDetail.type || "Concept" }}
            </span>
            <span class="rounded-md bg-slate-100 px-2 py-1 text-slate-600">
              状态：{{ selectedNodeDetail.status || "unknown" }}
            </span>
          </div>

          <div class="space-y-2">
            <p class="text-xs font-medium text-slate-500">原笔记中的对应描述</p>
            <template v-if="selectedNodeDetail.snippets.length">
              <pre
                v-for="(snippet, index) in selectedNodeDetail.snippets"
                :key="`${selectedNodeDetail.id}-${index}`"
                class="whitespace-pre-wrap rounded-lg border border-slate-200 bg-slate-50 p-2.5 text-xs leading-relaxed text-slate-700"
              >{{ snippet }}</pre>
            </template>
            <p v-else class="rounded-lg border border-dashed border-slate-200 bg-slate-50 p-2.5 text-xs text-slate-500">
              当前 Markdown 中未检索到该知识点的直接文本片段，可补充更详细笔记后重试。
            </p>
          </div>

          <div class="mt-3 rounded-lg border border-violet-200 bg-violet-50/60 p-2.5">
            <p class="text-xs font-medium text-violet-700">AI 智能体详细讲解</p>
            <p v-if="isExplaining" class="mt-1 text-xs text-violet-700">正在生成专业讲解...</p>
            <pre class="mt-1 whitespace-pre-wrap text-xs leading-relaxed text-slate-700">{{
              selectedNodeDetail.aiExplanation
            }}</pre>
          </div>
        </aside>
      </section>
    </div>

    <div
      v-if="isLoading"
      class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/30 backdrop-blur-[2px]"
    >
      <div class="rounded-2xl border border-white/20 bg-slate-900/90 px-8 py-6 text-center text-white shadow-2xl">
        <div class="mx-auto h-10 w-10 animate-spin rounded-full border-2 border-white/20 border-t-white" />
        <p class="mt-3 text-sm tracking-wide">AI 正在生成并融合知识图谱...</p>
      </div>
    </div>
  </main>
</template>
