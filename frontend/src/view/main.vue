<script setup lang="ts">
import { Graph } from "@antv/g6";
import { computed, onBeforeUnmount, onMounted, ref, nextTick } from "vue";
import {
  explainConcept, getGraphAll, getGraphPath, getNodeNeighbors,
  uploadNoteLangChain, listUserFiles, createFile, createFileGroup,
  deleteFile, deleteFileGroup, chatWithContext, getLearningPath,
  type GraphResponse, type UserFile, type FileGroup,
} from "../api/graph";
import {
  buildFocusSet, buildStyledGraph, preprocessGraphData,
  getLayoutConfig, getNodeConfig, getEdgeConfig, getCollideLayoutConfig,
  type LayoutType,
} from "../graph/g6-config";

// ==================== 响应式状态 ====================

const markdown = ref("");
const concept = ref("");
const userId = ref("");
const maxDepth = ref(3);
const isLoading = ref(false);
const isNavigating = ref(false);
const statusText = ref("图谱待生成");
const importedFileName = ref("");
const graphRoot = ref<HTMLDivElement | null>(null);

// 文件管理
const files = ref<UserFile[]>([]);
const fileGroups = ref<FileGroup[]>([]);
const selectedFileId = ref<string>("");
const selectedFileGroupId = ref<string>("");
const newFileName = ref("");
const newGroupName = ref("");

// 右侧面板
const selectedNodeDetail = ref<{
  id: string; label: string; type?: string; status?: string;
  reason?: string; snippets: string[]; aiExplanation: string;
} | null>(null);
const isExplaining = ref(false);

// AI 对话
const chatMessages = ref<Array<{ role: "user" | "ai"; content: string }>>([]);
const chatInput = ref("");
const isChatting = ref(false);

// LightRAG 模式
const isLightRAGMode = ref(true);
const expandedNodes = ref<Set<string>>(new Set());

let graph: Graph | null = null;
let graphRawData: GraphResponse = { nodes: [], edges: [] };
let resizeObserver: ResizeObserver | null = null;

const canGenerate = computed(() => markdown.value.trim().length > 0);
const canNavigate = computed(() => concept.value.trim().length > 0);

// ==================== 工具函数 ====================

function tooltipHtml(reason: string) {
  return `
    <div style="max-width:260px;border:1px solid #E2E8F0;border-radius:12px;
      background:rgba(15,23,42,0.95);color:#F8FAFC;padding:10px 12px;
      box-shadow:0 10px 30px rgba(15,23,42,0.25);font-size:12px;line-height:1.5;">
      ${reason || "暂无批注原因"}
    </div>`;
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
    const snippet = lines.slice(start, end + 1).join("\n").trim();
    if (snippet) snippets.push(snippet);
    if (snippets.length >= 3) break;
  }
  return snippets;
}

function buildAIExplanation(node: { label: string; status?: string; reason?: string }, snippets: string[]): string {
  const statusMap: Record<string, string> = {
    correct: "该知识点在当前笔记中逻辑基本成立。",
    error: "该知识点存在明显错误或概念混淆，需要优先纠正。",
    supplement: "该知识点存在逻辑断层，建议补全关键前置知识。",
  };
  const statusText = statusMap[node.status ?? ""] || "该知识点已被识别，但状态信息不足。";
  const snippetHint = snippets.length
    ? `基于原笔记可提取到 ${snippets.length} 处相关描述。`
    : "原笔记中未检索到明显原文描述。";
  const reason = node.reason?.trim() ? `系统批注：${node.reason.trim()}` : "";
  return `${statusText}\n\n${reason}\n\n${snippetHint}\n\n建议：1.写出概念定义 2.列出2个前置知识+1个应用场景 3.用自己的话复述。`;
}

function resolveNodeIdFromEvent(event: any): string {
  const candidates = [event?.data?.id, event?.target?.id, event?.target?.data?.id, event?.itemId];
  const hit = candidates.find((v) => typeof v === "string" && v.trim().length > 0);
  return hit ? String(hit) : "";
}

// ==================== 文件管理 ====================

async function loadFileList() {
  try {
    const result = await listUserFiles(userId.value.trim() || undefined);
    files.value = result.files ?? [];
    fileGroups.value = result.file_groups ?? [];
  } catch (_) { /* ignore */ }
}

async function handleCreateFile() {
  if (!newFileName.value.trim()) return;
  try {
    await createFile(newFileName.value.trim(), selectedFileGroupId.value || undefined, userId.value.trim() || undefined);
    newFileName.value = "";
    await loadFileList();
  } catch (err) { statusText.value = `创建文件失败：${(err as Error).message}`; }
}

async function handleCreateGroup() {
  if (!newGroupName.value.trim()) return;
  try {
    await createFileGroup(newGroupName.value.trim(), userId.value.trim() || undefined);
    newGroupName.value = "";
    await loadFileList();
  } catch (err) { statusText.value = `创建文件组失败：${(err as Error).message}`; }
}

async function handleDeleteFile(fileId: string) {
  try {
    await deleteFile(fileId, userId.value.trim() || undefined);
    if (selectedFileId.value === fileId) selectedFileId.value = "";
    await loadFileList();
  } catch (err) { statusText.value = `删除文件失败：${(err as Error).message}`; }
}

async function handleDeleteGroup(groupId: string) {
  try {
    await deleteFileGroup(groupId, userId.value.trim() || undefined);
    if (selectedFileGroupId.value === groupId) selectedFileGroupId.value = "";
    await loadFileList();
  } catch (err) { statusText.value = `删除文件组失败：${(err as Error).message}`; }
}

async function selectFile(fileId: string) {
  selectedFileId.value = fileId;
  selectedFileGroupId.value = "";
  await fetchGraphByFile(fileId);
}

async function selectFileGroup(groupId: string) {
  selectedFileGroupId.value = groupId;
  selectedFileId.value = "";
  await fetchGraphByGroup(groupId);
}

async function fetchGraphByFile(fileId: string) {
  try {
    const result = await getGraphAll({ file_id: fileId, user_id: userId.value.trim() || undefined });
    graphRawData = preprocessGraphData(result.nodes, result.edges, { minConfidence: 0.6, removeSelfLoops: true, keepIsolatedNodes: false });
    await renderGraph(graphRawData);
    statusText.value = `已加载文件图谱：${graphRawData.nodes.length} 节点 / ${graphRawData.edges.length} 连线`;
  } catch (err) { statusText.value = `加载失败：${(err as Error).message}`; }
}

async function fetchGraphByGroup(groupId: string) {
  try {
    const result = await getGraphAll({ file_group_id: groupId, user_id: userId.value.trim() || undefined });
    graphRawData = preprocessGraphData(result.nodes, result.edges, { minConfidence: 0.6, removeSelfLoops: true, keepIsolatedNodes: false });
    await renderGraph(graphRawData);
    statusText.value = `已加载文件组图谱：${graphRawData.nodes.length} 节点 / ${graphRawData.edges.length} 连线`;
  } catch (err) { statusText.value = `加载失败：${(err as Error).message}`; }
}

// ==================== AI 对话 ====================

async function sendChatMessage() {
  const msg = chatInput.value.trim();
  if (!msg || isChatting.value) return;
  chatMessages.value.push({ role: "user", content: msg });
  chatInput.value = "";
  isChatting.value = true;

  try {
    const graphNodes = JSON.stringify(graphRawData.nodes.map(n => ({
      name: n.label || n.id, status: n.status, reason: n.reason, definition: n.data?.definition ?? ""
    })));
    const graphEdges = JSON.stringify(graphRawData.edges.map(e => ({
      source: e.source, target: e.target, relation: e.label, status: e.status, reason: e.reason
    })));
    const result = await chatWithContext({
      user_message: msg,
      graph_nodes: graphNodes,
      graph_edges: graphEdges,
    });
    chatMessages.value.push({ role: "ai", content: result.reply });
  } catch (err) {
    chatMessages.value.push({ role: "ai", content: `抱歉，对话出错：${(err as Error).message}` });
  } finally {
    isChatting.value = false;
    await nextTick();
    scrollChatBottom();
  }
}

function scrollChatBottom() {
  const el = document.getElementById("chat-messages");
  if (el) el.scrollTop = el.scrollHeight;
}

// ==================== 节点详情与讲解 ====================

async function showNodeDetail(nodeId: string) {
  const node = graphRawData.nodes.find((item) => item.id === nodeId);
  if (!node) return;
  const snippets = extractMarkdownSnippets(node.label || node.id, markdown.value);
  selectedNodeDetail.value = {
    id: node.id, label: node.label || node.id,
    type: node.type || String(node.data?.type ?? ""),
    status: node.status || String(node.data?.status ?? ""),
    reason: node.reason || String(node.data?.reason ?? ""),
    snippets,
    aiExplanation: buildAIExplanation(
      { label: node.label || node.id, status: node.status, reason: node.reason }, snippets
    ),
  };

  if (!markdown.value.trim()) return;
  isExplaining.value = true;
  try {
    const result = await explainConcept({ concept: node.label || node.id, markdown: markdown.value, user_id: userId.value.trim() || undefined });
    if (selectedNodeDetail.value?.id === node.id) {
      selectedNodeDetail.value.aiExplanation = result.explanation;
    }
  } catch (error) {
    if (selectedNodeDetail.value?.id === node.id) {
      selectedNodeDetail.value.aiExplanation = `讲解生成失败：${(error as Error).message}`;
    }
  } finally { isExplaining.value = false; }
}

// ==================== 图谱渲染 ====================

async function initGraph() {
  if (!graphRoot.value) return;
  const { width, height } = graphRoot.value.getBoundingClientRect();
  graph = new Graph({
    container: graphRoot.value,
    width, height,
    autoFit: "view",
    data: { nodes: [], edges: [] },
    node: getNodeConfig(),
    edge: getEdgeConfig(),
    layout: getLayoutConfig("force"),
    behaviors: ["drag-canvas", "zoom-canvas", "drag-element"],
    plugins: [{
      type: "tooltip",
      trigger: "hover",
      enable: (event: { targetType?: string; target?: { data?: Record<string, unknown> } }) => {
        if (event.targetType !== "node") return false;
        const status = String(event.target?.data?.status ?? "");
        return status === "error" || status === "supplement";
      },
      getContent: (event: { target?: { data?: Record<string, unknown> } }) =>
        tooltipHtml(String(event.target?.data?.reason ?? "")),
    }],
  });
  await graph.render();

  // 点击节点
  graph.on("node:click", (event: any) => {
    const nodeId = resolveNodeIdFromEvent(event);
    if (!nodeId) return;
    if (isLightRAGMode.value && !expandedNodes.value.has(nodeId)) {
      void expandNodeNeighbors(nodeId);
    } else {
      void showNodeDetail(nodeId);
    }
  });
  graph.on("element:click", (event: any) => {
    if (event?.targetType !== "node") return;
    const nodeId = resolveNodeIdFromEvent(event);
    if (!nodeId) return;
    if (isLightRAGMode.value && !expandedNodes.value.has(nodeId)) {
      void expandNodeNeighbors(nodeId);
    } else {
      void showNodeDetail(nodeId);
    }
  });
  graph.on("canvas:click", () => { selectedNodeDetail.value = null; });
}

async function renderGraph(data: GraphResponse, focusMode = false, pathData: GraphResponse[] = [], layoutType: LayoutType = "force") {
  if (!graph) return;
  const focusSet = focusMode ? buildFocusSet(pathData) : undefined;
  const styled = buildStyledGraph(data, focusSet);
  const hasPresetPositions = styled.nodes.some((n: any) => n.x !== undefined && n.y !== undefined);
  graph.setLayout(hasPresetPositions ? { type: "preset", padding: 50 } : getLayoutConfig(layoutType));
  graph.setData(styled as any);
  await graph.render();
}

async function expandNodeNeighbors(nodeId: string) {
  if (!graph) return;
  statusText.value = `正在展开节点 "${nodeId}" 的邻居...`;
  try {
    const neighbors = await getNodeNeighbors(nodeId, userId.value.trim() || undefined, 1);
    const existingNodeIds = new Set(graphRawData.nodes.map((n) => n.id));
    const existingEdgeIds = new Set(graphRawData.edges.map((e) => e.id));
    const newNodes = neighbors.nodes.filter((n) => !existingNodeIds.has(n.id));
    const newEdges = neighbors.edges.filter((e) => !existingEdgeIds.has(e.id));
    if (newNodes.length === 0 && newEdges.length === 0) {
      statusText.value = `节点 "${nodeId}" 没有更多邻居`;
      expandedNodes.value.add(nodeId);
      return;
    }
    graphRawData.nodes.push(...newNodes);
    graphRawData.edges.push(...newEdges);
    expandedNodes.value.add(nodeId);
    const styled = buildStyledGraph({ nodes: newNodes, edges: newEdges });
    graph.addData(styled as any);
    await graph.render();
    statusText.value = `已展开 ${newNodes.length} 个节点，${newEdges.length} 条边`;
  } catch (error) { statusText.value = `展开邻居失败：${(error as Error).message}`; }
}

async function fetchAllGraph() {
  graphRawData = { nodes: [], edges: [] };
  if (selectedFileId.value) {
    await fetchGraphByFile(selectedFileId.value);
    return;
  }
  if (selectedFileGroupId.value) {
    await fetchGraphByGroup(selectedFileGroupId.value);
    return;
  }
  // 默认加载全部
  const rawData = await getGraphAll({ user_id: userId.value.trim() || undefined });
  graphRawData = preprocessGraphData(rawData.nodes, rawData.edges, { minConfidence: 0.6, removeSelfLoops: true, keepIsolatedNodes: false });
  if (isLightRAGMode.value) {
    graphRawData = extractCoreNodes(graphRawData, 5);
    expandedNodes.value.clear();
  }
  await renderGraph(graphRawData);
  statusText.value = `图谱已加载：${graphRawData.nodes.length} 节点 / ${graphRawData.edges.length} 连线${isLightRAGMode.value ? " (渐进式展开)" : ""}`;
}

function extractCoreNodes(data: GraphResponse, count: number): GraphResponse {
  const nodeDegree = new Map<string, number>();
  data.nodes.forEach((node) => nodeDegree.set(node.id, 0));
  data.edges.forEach((edge) => {
    nodeDegree.set(edge.source, (nodeDegree.get(edge.source) || 0) + 1);
    nodeDegree.set(edge.target, (nodeDegree.get(edge.target) || 0) + 1);
  });
  const sortedNodes = [...data.nodes].sort((a, b) => (nodeDegree.get(b.id) || 0) - (nodeDegree.get(a.id) || 0)).slice(0, count);
  const coreNodeIds = new Set(sortedNodes.map((n) => n.id));
  const coreEdges = data.edges.filter((e) => coreNodeIds.has(e.source) && coreNodeIds.has(e.target));
  return { nodes: sortedNodes, edges: coreEdges };
}

// ==================== 图谱生成/导航 ====================

async function handleUpload() {
  if (!canGenerate.value) return;
  isLoading.value = true;
  statusText.value = "AI 正在解析诊断（LangChain 三 Agent 流水线）...";
  try {
    await uploadNoteLangChain({
      markdown: markdown.value,
      user_id: userId.value.trim() || undefined,
      file_id: selectedFileId.value || undefined,
      file_group_id: selectedFileGroupId.value || undefined,
    });
    await fetchAllGraph();
    isNavigating.value = false;
    selectedNodeDetail.value = null;
  } catch (error) {
    statusText.value = `生成失败：${(error as Error).message}`;
  } finally { isLoading.value = false; }
}

async function handleImportMarkdownFile(event: Event) {
  const input = event.target as HTMLInputElement | null;
  const file = input?.files?.[0];
  if (!file) return;
  if (!file.name.toLowerCase().endsWith(".md") && file.type !== "text/markdown") {
    statusText.value = "仅支持导入 .md Markdown 文件"; input.value = ""; return;
  }
  if (file.size > 2 * 1024 * 1024) {
    statusText.value = "Markdown 文件过大，请控制在 2MB 以内"; input.value = ""; return;
  }
  try {
    markdown.value = await file.text();
    importedFileName.value = file.name;
    statusText.value = `已导入文件：${file.name}`;
  } catch (error) { statusText.value = `读取文件失败：${(error as Error).message}`; }
  finally { input.value = ""; }
}

async function handlePathNavigate() {
  if (!canNavigate.value) return;
  statusText.value = "正在计算逆向学习路径...";
  isNavigating.value = true;
  try {
    const result = await getGraphPath(concept.value.trim(), userId.value.trim() || undefined, maxDepth.value);

    // 如果有关联图谱数据，用专注模式渲染
    if (result.all_related) {
      await renderGraph(result.all_related, true, [result.all_related], "dagre");
    } else {
      await renderGraph(graphRawData, true, result.paths, "dagre");
    }

    // 如果有依赖树数据，也触发 AI 学习路径指导
    if (result.dependency_tree && result.dependency_tree.length > 0) {
      const guidance = await getLearningPath({
        target_concept: concept.value.trim(),
        dependency_tree_json: JSON.stringify(result.dependency_tree),
        graph_nodes_json: JSON.stringify(result.all_related?.nodes ?? graphRawData.nodes),
      }).catch(() => null);
      if (guidance?.guidance) {
        chatMessages.value.push({ role: "ai", content: guidance.guidance });
        await nextTick();
        scrollChatBottom();
      }
    }

    statusText.value = `专注模式：命中 ${result.paths.length} 条依赖路径，${result.dependency_tree?.length ?? 0} 个前置节点`;
  } catch (error) {
    statusText.value = `路径查询失败：${(error as Error).message}`;
  }
}

async function resetFocus() {
  await renderGraph(graphRawData, false, [], "force");
  isNavigating.value = false;
  statusText.value = "已退出专注模式";
}

// ==================== 生命周期 ====================

onMounted(async () => {
  await initGraph();
  await loadFileList();
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
  <main class="flex h-screen w-full overflow-hidden bg-slate-100">
    <!-- ====== 左侧面板：文件管理 ====== -->
    <aside class="flex w-[260px] shrink-0 flex-col border-r border-slate-200 bg-white">
      <div class="border-b border-slate-200 px-4 py-3">
        <h2 class="text-sm font-semibold text-slate-800">📁 文件管理</h2>
        <p class="mt-0.5 text-xs text-slate-400">文件组（融合图谱）/ 独立文件</p>
      </div>

      <!-- 文件组列表 -->
      <div class="border-b border-slate-100 px-3 py-2">
        <div class="mb-2 flex items-center gap-1">
          <input v-model="newGroupName" placeholder="新建文件组..." class="flex-1 rounded-lg border border-slate-200 px-2 py-1.5 text-xs outline-none focus:border-violet-400" @keyup.enter="handleCreateGroup" />
          <button class="shrink-0 rounded-lg bg-violet-100 px-2 py-1.5 text-xs font-medium text-violet-700 hover:bg-violet-200" @click="handleCreateGroup">+</button>
        </div>
        <div class="max-h-[30vh] space-y-1 overflow-y-auto">
          <div v-for="group in fileGroups" :key="group.id"
            :class="['flex items-center justify-between rounded-lg px-2 py-1.5 text-xs cursor-pointer transition',
              selectedFileGroupId === group.id ? 'bg-violet-100 text-violet-800 font-medium' : 'hover:bg-slate-50 text-slate-700']"
            @click="selectFileGroup(group.id)">
            <span>📦 {{ group.name }} <span class="text-slate-400">({{ group.file_ids?.length ?? 0 }})</span></span>
            <button class="text-slate-400 hover:text-red-500" @click.stop="handleDeleteGroup(group.id)">×</button>
          </div>
          <p v-if="fileGroups.length === 0" class="px-2 text-xs text-slate-400">暂无文件组</p>
        </div>
      </div>

      <!-- 独立文件列表 -->
      <div class="flex-1 overflow-y-auto px-3 py-2">
        <div class="mb-2 flex items-center gap-1">
          <input v-model="newFileName" placeholder="新建文件..." class="flex-1 rounded-lg border border-slate-200 px-2 py-1.5 text-xs outline-none focus:border-blue-400" @keyup.enter="handleCreateFile" />
          <button class="shrink-0 rounded-lg bg-blue-100 px-2 py-1.5 text-xs font-medium text-blue-700 hover:bg-blue-200" @click="handleCreateFile">+</button>
        </div>
        <div class="space-y-1">
          <div v-for="file in files.filter(f => !f.file_group_id)" :key="file.id"
            :class="['flex items-center justify-between rounded-lg px-2 py-1.5 text-xs cursor-pointer transition',
              selectedFileId === file.id ? 'bg-blue-100 text-blue-800 font-medium' : 'hover:bg-slate-50 text-slate-700']"
            @click="selectFile(file.id)">
            <span>📄 {{ file.name }}</span>
            <button class="text-slate-400 hover:text-red-500" @click.stop="handleDeleteFile(file.id)">×</button>
          </div>
          <p v-if="files.filter(f => !f.file_group_id).length === 0" class="px-2 text-xs text-slate-400">暂无独立文件</p>
        </div>
      </div>

      <!-- 文件组内的文件 -->
      <div v-if="selectedFileGroupId" class="border-t border-slate-200 px-3 py-2">
        <p class="mb-1 text-xs font-medium text-slate-500">组内文件</p>
        <div class="space-y-1">
          <div v-for="file in files.filter(f => f.file_group_id === selectedFileGroupId)" :key="file.id"
            class="flex items-center justify-between rounded-lg px-2 py-1.5 text-xs bg-violet-50 text-violet-700 cursor-pointer hover:bg-violet-100"
            @click="selectFile(file.id)">
            <span>📄 {{ file.name }}</span>
            <button class="text-slate-400 hover:text-red-500" @click.stop="handleDeleteFile(file.id)">×</button>
          </div>
        </div>
      </div>

      <!-- 底部刷新按钮 -->
      <div class="border-t border-slate-200 px-3 py-2">
        <button class="w-full rounded-lg bg-slate-100 py-1.5 text-xs text-slate-600 hover:bg-slate-200" @click="loadFileList">🔄 刷新文件列表</button>
      </div>
    </aside>

    <!-- ====== 中间：图谱 + 笔记输入 ====== -->
    <section class="flex flex-1 flex-col overflow-hidden">
      <!-- 顶部工具栏 -->
      <div class="flex items-center gap-3 border-b border-slate-200 bg-white px-4 py-2.5">
        <h1 class="text-sm font-semibold text-slate-800 shrink-0">Knowledge Studio</h1>
        <div class="h-4 w-px bg-slate-200" />
        <label class="inline-flex cursor-pointer items-center rounded-lg border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs text-slate-600 transition hover:bg-white">
          导入 .md
          <input class="hidden" type="file" accept=".md,text/markdown" @change="handleImportMarkdownFile" />
        </label>
        <span v-if="importedFileName" class="truncate text-xs text-slate-500">{{ importedFileName }}</span>
        <div class="flex-1" />
        <button
          :class="['rounded-lg px-3 py-1.5 text-xs font-medium transition', isLightRAGMode ? 'bg-emerald-100 text-emerald-700' : 'bg-slate-100 text-slate-600']"
          @click="isLightRAGMode = !isLightRAGMode; fetchAllGraph()">
          {{ isLightRAGMode ? '渐进式展开' : '显示全图' }}
        </button>
        <span class="text-xs text-slate-400">{{ statusText }}</span>
      </div>

      <!-- 图谱区域 + 输入区 -->
      <div class="flex flex-1 flex-col overflow-hidden p-3">
        <!-- 图谱 -->
        <div ref="graphRoot" class="flex-1 rounded-xl border border-slate-200 bg-white shadow-sm min-h-[300px]" />

        <!-- 笔记输入 + 导航 -->
        <div class="mt-3 flex gap-3">
          <div class="flex flex-1 flex-col gap-2">
            <textarea v-model="markdown"
              class="h-28 w-full resize-none rounded-xl border border-slate-200 bg-white p-3 text-sm text-slate-700 outline-none transition focus:border-blue-400"
              placeholder="粘贴 Markdown 笔记..." />
            <div class="flex gap-2">
              <input v-model="userId" class="w-32 rounded-lg border border-slate-200 px-2 py-1.5 text-xs outline-none" placeholder="user_id" />
              <button :disabled="!canGenerate || isLoading"
                class="rounded-lg bg-slate-900 px-4 py-1.5 text-xs font-medium text-white hover:bg-slate-800 disabled:opacity-50"
                @click="handleUpload">
                一键生成/体检知识图谱
              </button>
            </div>
          </div>
          <div class="flex w-[220px] shrink-0 flex-col gap-2">
            <input v-model="concept" class="rounded-lg border border-slate-200 px-2 py-1.5 text-xs outline-none focus:border-violet-400" placeholder="目标概念" />
            <input v-model.number="maxDepth" type="number" min="1" max="6" class="rounded-lg border border-slate-200 px-2 py-1.5 text-xs outline-none" placeholder="深度(3)" />
            <button :disabled="!canNavigate" class="rounded-lg bg-violet-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-violet-500 disabled:opacity-50" @click="handlePathNavigate">
              逆向学习导航
            </button>
            <button v-if="isNavigating" class="rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs text-slate-600 hover:bg-slate-50" @click="resetFocus">
              退出专注模式
            </button>
          </div>
        </div>
      </div>
    </section>

    <!-- ====== 右侧面板：AI 对话 + 节点详情 ====== -->
    <aside class="flex w-[340px] shrink-0 flex-col border-l border-slate-200 bg-white">
      <!-- 节点详情 -->
      <div v-if="selectedNodeDetail" class="border-b border-slate-200 p-3">
        <div class="mb-2 flex items-center justify-between">
          <h3 class="text-sm font-semibold text-slate-800 truncate">{{ selectedNodeDetail.label }}</h3>
          <button class="text-xs text-slate-400 hover:text-slate-600" @click="selectedNodeDetail = null">×</button>
        </div>
        <div class="flex gap-1.5 mb-2 text-xs">
          <span :class="['rounded-md px-1.5 py-0.5 font-medium',
            selectedNodeDetail.status === 'error' ? 'bg-red-100 text-red-700' :
            selectedNodeDetail.status === 'supplement' ? 'bg-violet-100 text-violet-700' : 'bg-slate-100 text-slate-600']">
            {{ selectedNodeDetail.status || 'unknown' }}
          </span>
          <span class="rounded-md bg-slate-100 px-1.5 py-0.5 text-slate-600">{{ selectedNodeDetail.type || 'Concept' }}</span>
        </div>
        <div v-if="selectedNodeDetail.reason" class="mb-2 rounded-lg bg-amber-50 p-2 text-xs text-amber-800">
          💡 {{ selectedNodeDetail.reason }}
        </div>
        <div v-if="selectedNodeDetail.snippets.length" class="mb-2 space-y-1">
          <p class="text-xs font-medium text-slate-500">原文描述</p>
          <pre v-for="(s, i) in selectedNodeDetail.snippets" :key="i"
            class="whitespace-pre-wrap rounded border border-slate-200 bg-slate-50 p-1.5 text-xs text-slate-700">{{ s }}</pre>
        </div>
        <div class="rounded-lg border border-violet-200 bg-violet-50/60 p-2">
          <p class="text-xs font-medium text-violet-700">AI 讲解</p>
          <p v-if="isExplaining" class="text-xs text-violet-500">生成中...</p>
          <pre v-else class="mt-1 whitespace-pre-wrap text-xs text-slate-700">{{ selectedNodeDetail.aiExplanation }}</pre>
        </div>
      </div>

      <!-- AI 对话 -->
      <div class="flex flex-1 flex-col overflow-hidden">
        <div class="border-b border-slate-200 px-3 py-2.5">
          <h2 class="text-sm font-semibold text-slate-800">🤖 AI 学习导师</h2>
          <p class="text-xs text-slate-400">基于当前图谱上下文回答</p>
        </div>
        <div id="chat-messages" class="flex-1 space-y-2 overflow-y-auto p-3">
          <div v-if="chatMessages.length === 0" class="py-8 text-center text-xs text-slate-400">
            上传笔记生成图谱后，AI 导师可基于图谱内容回答你的问题。
          </div>
          <div v-for="(msg, i) in chatMessages" :key="i"
            :class="['rounded-xl px-3 py-2 text-xs max-w-[90%]', msg.role === 'user' ? 'ml-auto bg-blue-600 text-white' : 'bg-slate-100 text-slate-700']">
            {{ msg.content }}
          </div>
          <div v-if="isChatting" class="rounded-xl bg-slate-100 px-3 py-2 text-xs text-slate-400">AI 思考中...</div>
        </div>
        <div class="border-t border-slate-200 p-2.5 flex gap-2">
          <input v-model="chatInput" @keyup.enter="sendChatMessage"
            class="flex-1 rounded-lg border border-slate-200 px-2.5 py-1.5 text-xs outline-none focus:border-blue-400"
            placeholder="输入消息..." :disabled="isChatting" />
          <button @click="sendChatMessage" :disabled="!chatInput.trim() || isChatting"
            class="rounded-lg bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-500 disabled:opacity-50">
            发送
          </button>
        </div>
      </div>
    </aside>

    <!-- 加载遮罩 -->
    <div v-if="isLoading" class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/30 backdrop-blur-[2px]">
      <div class="rounded-2xl border border-white/20 bg-slate-900/90 px-8 py-6 text-center text-white shadow-2xl">
        <div class="mx-auto h-10 w-10 animate-spin rounded-full border-2 border-white/20 border-t-white" />
        <p class="mt-3 text-sm tracking-wide">AI 正在生成并融合知识图谱...</p>
      </div>
    </div>
  </main>
</template>
