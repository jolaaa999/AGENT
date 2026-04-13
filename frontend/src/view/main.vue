<script setup lang="ts">
import { Graph } from "@antv/g6";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { explainConcept, getGraphAll, getGraphPath, uploadNote, type GraphResponse } from "../api/graph";
import { buildFocusSet, buildStyledGraph } from "../graph/g6-config";

const markdown = ref("");
const concept = ref("");
const userId = ref("");
const maxDepth = ref(3);
const isLoading = ref(false);
const isNavigating = ref(false);
const statusText = ref("图谱待生成");
const importedFileName = ref("");
const graphRoot = ref<HTMLDivElement | null>(null);
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

async function initGraph() {
  if (!graphRoot.value) return;

  const { width, height } = graphRoot.value.getBoundingClientRect();
  graph = new Graph({
    container: graphRoot.value,
    width,
    height,
    autoFit: "view",
    data: { nodes: [], edges: [] },
    node: {
      type: "circle",
      style: {
        size: 54
      }
    },
    edge: {
      type: "line"
    },
    layout: {
      type: "force",
      preventOverlap: true,
      nodeSize: 58,
      linkDistance: 140
    },
    behaviors: ["drag-canvas", "zoom-canvas", "drag-element"],
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
  });

  await graph.render();

  graph.on("node:click", (event: any) => {
    const nodeId = resolveNodeIdFromEvent(event);
    if (!nodeId) return;
    void showNodeDetail(nodeId);
  });

  graph.on("element:click", (event: any) => {
    if (event?.targetType !== "node") return;
    const nodeId = resolveNodeIdFromEvent(event);
    if (!nodeId) return;
    void showNodeDetail(nodeId);
  });

  graph.on("canvas:click", () => {
    selectedNodeDetail.value = null;
  });
}

async function renderGraph(data: GraphResponse, focusMode = false, pathData: GraphResponse[] = []) {
  if (!graph) return;

  const focusSet = focusMode ? buildFocusSet(pathData) : undefined;
  const styled = buildStyledGraph(data, focusSet);
  graph.setData(styled as any);
  await graph.render();
}

async function fetchAllGraph() {
  graphRawData = await getGraphAll(userId.value.trim() || undefined);
  await renderGraph(graphRawData);
  statusText.value = `图谱已加载：${graphRawData.nodes.length} 节点 / ${graphRawData.edges.length} 连线`;
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
    await renderGraph(graphRawData, true, result.paths);
    isNavigating.value = true;
    statusText.value = `专注模式已开启：命中 ${result.paths.length} 条依赖路径`;
  } catch (error) {
    statusText.value = `路径查询失败：${(error as Error).message}`;
  }
}

async function resetFocus() {
  await renderGraph(graphRawData);
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
