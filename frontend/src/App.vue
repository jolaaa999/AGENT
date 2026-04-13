<script setup lang="ts">
import { Graph } from "@antv/g6";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { getGraphAll, getGraphPath, uploadNote, type GraphResponse } from "./api/graph";
import { buildFocusSet, buildStyledGraph } from "./graph/g6-config";

const markdown = ref("");
const concept = ref("");
const userId = ref("");
const maxDepth = ref(3);
const isLoading = ref(false);
const isNavigating = ref(false);
const statusText = ref("图谱待生成");
const graphRoot = ref<HTMLDivElement | null>(null);

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
  } catch (error) {
    statusText.value = `生成失败：${(error as Error).message}`;
  } finally {
    isLoading.value = false;
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
    <div class="mx-auto grid max-w-[1500px] grid-cols-1 gap-5 xl:grid-cols-[420px_1fr]">
      <section class="rounded-2xl border border-slate-200/80 bg-white/95 p-6 shadow-[0_10px_40px_rgba(15,23,42,0.05)]">
        <header class="mb-5">
          <h1 class="text-[22px] font-semibold tracking-tight text-slate-900">Knowledge Studio</h1>
          <p class="mt-2 text-sm leading-relaxed text-slate-500">
            粘贴 Markdown，生成可解释的知识图谱，并对目标概念执行逆向导航学习。
          </p>
        </header>

        <label class="mb-2 block text-xs font-medium uppercase tracking-[0.12em] text-slate-400">
          Markdown 笔记输入
        </label>
        <textarea
          v-model="markdown"
          class="h-72 w-full resize-none rounded-xl border border-slate-200 bg-slate-50/70 p-4 text-sm leading-relaxed text-slate-700 outline-none transition focus:border-blue-400 focus:bg-white"
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

      <section class="relative overflow-hidden rounded-2xl border border-slate-200/80 bg-white shadow-[0_10px_40px_rgba(15,23,42,0.05)]">
        <div class="flex items-center justify-between border-b border-slate-200 px-5 py-3.5">
          <p class="text-sm font-medium text-slate-700">图谱可视化区</p>
          <p class="text-xs text-slate-500">{{ statusText }}</p>
        </div>
        <div ref="graphRoot" class="h-[720px] w-full bg-[radial-gradient(circle_at_20%_20%,#f8fafc,transparent_45%),radial-gradient(circle_at_80%_90%,#eef2ff,transparent_40%)]" />
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
