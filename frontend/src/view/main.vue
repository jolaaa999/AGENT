<script setup lang="ts">
import { Graph } from "@antv/g6";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { marked } from "marked";
import {
  addFileToGroup,
  chatWithContext,
  createFileGroup,
  deleteConversation,
  deleteFile,
  deleteFileGroup,
  explainConcept,
  getConversation,
  getGraphAll,
  getGraphPath,
  getLearningPath,
  getNodeNeighbors,
  listUserFiles,
  renameFile,
  renameFileGroup,
  saveMessage,
  togglePinFile,
  togglePinFileGroup,
  uploadNoteLangChain,
  type FileGroup,
  type GraphResponse,
  type UserFile,
} from "../api/graph";
import {
  buildFocusSet,
  buildStyledGraph,
  getEdgeConfig,
  getLayoutConfig,
  getNodeConfig,
  preprocessGraphData,
  type LayoutType,
} from "../graph/g6-config";
import FileSidebar from "../components/FileSidebar.vue";
import ImportPanel from "../components/ImportPanel.vue";
import LearningNavPanel from "../components/LearningNavPanel.vue";
import AiChatPanel from "../components/AiChatPanel.vue";

function renderMarkdown(text: string): string {
  if (!text) return "";
  return marked.parse(text, { breaks: true }) as string;
}

const markdown = ref("");
const concept = ref("");
const userId = ref("");
const loggedInUserId = ref("");
const maxDepth = ref(3);
const isLoading = ref(false);
const isNavigating = ref(false);
const statusText = ref("图谱待生成");
const importedFileName = ref("");
const graphRoot = ref<HTMLDivElement | null>(null);
const selectedFileId = ref("");
const selectedFileGroupId = ref("");

const files = ref<UserFile[]>([]);
const fileGroups = ref<FileGroup[]>([]);
const newGroupName = ref("");
const menuOpen = ref("");
const renameTarget = ref<{ type: "file" | "group"; id: string; currentName: string } | null>(null);
const renameValue = ref("");
const addFileToGroupTarget = ref("");
const showLoginPrompt = ref(false);
const isSwitching = ref(false);
const isRefreshing = ref(false);

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

const chatMessages = ref<Array<{ role: "user" | "ai"; content: string }>>([]);
const chatInput = ref("");
const isChatting = ref(false);
const currentConversationId = ref("");

const isLightRAGMode = ref(true);
const expandedNodes = ref<Set<string>>(new Set());
const graphSearch = ref("");
const graphStatusFilter = ref<"all" | "correct" | "error" | "supplement">("all");
const graphLayoutMode = ref<"auto" | LayoutType>("auto");
const activeLayout = ref<LayoutType>("force");
const isFocusMode = ref(false);
const focusedNodeId = ref("");

const leftWidth = ref(280);
const rightWidth = ref(340);
const dragging = ref<"left" | "right" | null>(null);

function onDividerMousedown(side: "left" | "right", e: MouseEvent) {
  dragging.value = side;
  e.preventDefault();
  document.body.style.cursor = "col-resize";
  document.body.style.userSelect = "none";
}

function onDividerMousemove(e: MouseEvent) {
  if (!dragging.value) return;
  const minW = 200;
  const maxW = 480;
  if (dragging.value === "left") {
    leftWidth.value = Math.min(maxW, Math.max(minW, e.clientX));
  } else {
    rightWidth.value = Math.min(maxW, Math.max(minW, window.innerWidth - e.clientX));
  }
}

function onDividerMouseup() {
  if (!dragging.value) return;
  dragging.value = null;
  document.body.style.cursor = "";
  document.body.style.userSelect = "";
  resizeGraph();
}

function resizeGraph() {
  requestAnimationFrame(() => {
    if (graphRoot.value && graph) {
      const r = graphRoot.value.getBoundingClientRect();
      graph.resize(r.width, r.height);
    }
  });
}

window.addEventListener("mousemove", onDividerMousemove);
window.addEventListener("mouseup", onDividerMouseup);

let graph: Graph | null = null;
let graphRawData: GraphResponse = { nodes: [], edges: [] };
let resizeObserver: ResizeObserver | null = null;

const canGenerate = computed(() => markdown.value.trim().length > 0);
const canNavigate = computed(() => concept.value.trim().length > 0);

function currentUserId(): string | undefined {
  return loggedInUserId.value || undefined;
}

function requireLogin(): boolean {
  if (!loggedInUserId.value) {
    showLoginPrompt.value = true;
    return false;
  }
  return true;
}

function tooltipHtml(reason: string) {
  return `<div style="max-width:260px;border:1px solid #E2E8F0;border-radius:12px;background:rgba(15,23,42,0.95);color:#F8FAFC;padding:10px 12px;box-shadow:0 10px 30px rgba(15,23,42,0.25);font-size:12px;line-height:1.5;">${reason || "暂无批注原因"}</div>`;
}

function extractMarkdownSnippets(conceptName: string, content: string): string[] {
  if (!conceptName.trim() || !content.trim()) return [];
  const lines = content.split(/\r?\n/);
  const nc = conceptName.toLowerCase();
  const snippets: string[] = [];
  const used = new Set<string>();
  for (let i = 0; i < lines.length; i++) {
    if (!lines[i].toLowerCase().includes(nc)) continue;
    const s = Math.max(0, i - 1);
    const e = Math.min(lines.length - 1, i + 1);
    const k = `${s}-${e}`;
    if (used.has(k)) continue;
    used.add(k);
    const sn = lines.slice(s, e + 1).join("\n").trim();
    if (sn) snippets.push(sn);
    if (snippets.length >= 3) break;
  }
  return snippets;
}

function buildAIExplanation(
  node: { label: string; status?: string; reason?: string },
  snippets: string[],
): string {
  const m: Record<string, string> = {
    correct: "该知识点逻辑基本成立。",
    error: "该知识点存在明显错误，需优先纠正。",
    supplement: "该知识点存在逻辑断层，建议补全前置知识。",
  };
  const st = m[node.status ?? ""] || "状态信息不足。";
  const sh = snippets.length
    ? `基于原笔记可提取到 ${snippets.length} 处相关描述。`
    : "未检索到明显原文描述。";
  const r = node.reason?.trim() ? `系统批注：${node.reason.trim()}` : "";
  return `${st}\n\n${r}\n\n${sh}\n\n建议：1.写出概念定义 2.列出2个前置知识+1个应用场景 3.用自己的话复述。`;
}

function resolveNodeIdFromEvent(event: any): string {
  const c = [event?.data?.id, event?.target?.id, event?.target?.data?.id, event?.itemId];
  const h = c.find((v: any) => typeof v === "string" && v.trim().length > 0);
  if (h) return String(h);

  const raw = event?.target?.data ?? event?.data;
  if (raw?.nodeType && typeof raw?.id !== "undefined") return String(raw.id);
  return "";
}

async function handleLogin() {
  const u = userId.value.trim();
  if (!u) return;
  loggedInUserId.value = u;
  selectedFileId.value = "";
  selectedFileGroupId.value = "";
  chatMessages.value = [];
  currentConversationId.value = "";
  showLoginPrompt.value = false;
  await loadFileList();
  statusText.value =
    files.value.length === 0 && fileGroups.value.length === 0
      ? `新用户「${u}」已创建，上传 MD 文件开始使用`
      : `欢迎回来「${u}」，${files.value.length} 个文件、${fileGroups.value.length} 个文件组`;
}

async function loadFileList(options?: { silent?: boolean }) {
  try {
    const r = await listUserFiles(currentUserId());
    files.value = r.files ?? [];
    fileGroups.value = r.file_groups ?? [];
  } catch (err) {
    if (!options?.silent) {
      throw err;
    }
  }
}

async function handleRefresh() {
  if (isRefreshing.value) return;

  isRefreshing.value = true;
  statusText.value = "正在刷新…";

  try {
    await loadFileList();

    if (selectedFileId.value && !files.value.some((f) => f.id === selectedFileId.value)) {
      selectedFileId.value = "";
    }
    if (selectedFileGroupId.value && !fileGroups.value.some((g) => g.id === selectedFileGroupId.value)) {
      selectedFileGroupId.value = "";
    }

    await fetchAllGraph();

    statusText.value = `已刷新：${files.value.length} 个文件、${fileGroups.value.length} 个文件组`;
  } catch (err) {
    statusText.value = `刷新失败：${(err as Error).message}`;
  } finally {
    isRefreshing.value = false;
  }
}

async function handleCreateGroup() {
  const name = newGroupName.value.trim();
  if (!name || !requireLogin()) return;
  if (fileGroups.value.some((group) => group.name.trim().toLowerCase() === name.toLowerCase())) {
    statusText.value = `文件组「${name}」已存在`;
    return;
  }
  try {
    const created = await createFileGroup(name, currentUserId());
    newGroupName.value = "";
    await loadFileList();
    selectedFileGroupId.value = created.group_id;
    selectedFileId.value = "";
    menuOpen.value = "";
    statusText.value = `已创建文件组「${name}」`;
  } catch (err) {
    statusText.value = `创建文件组失败：${(err as Error).message}`;
  }
}

async function handleDeleteFile(id: string) {
  try {
    await deleteFile(id, currentUserId());
    if (selectedFileId.value === id) selectedFileId.value = "";
    menuOpen.value = "";
    await loadFileList();
  } catch (err) {
    statusText.value = `删除失败：${(err as Error).message}`;
  }
}

async function handleDeleteGroup(id: string) {
  const group = fileGroups.value.find((item) => item.id === id);
  const fileCount = files.value.filter((file) => file.file_group_id === id).length;
  const message = fileCount
    ? `确定删除文件组「${group?.name ?? id}」吗？组内 ${fileCount} 个文件也可能被删除。`
    : `确定删除空文件组「${group?.name ?? id}」吗？`;
  if (!window.confirm(message)) return;
  try {
    await deleteFileGroup(id, currentUserId());
    if (selectedFileGroupId.value === id) {
      selectedFileGroupId.value = "";
      graphRawData = { nodes: [], edges: [] };
      await renderGraph(graphRawData);
    }
    menuOpen.value = "";
    await loadFileList();
    statusText.value = `已删除文件组「${group?.name ?? id}」`;
  } catch (err) {
    statusText.value = `删除失败：${(err as Error).message}`;
  }
}

function openRenameDialog(type: "file" | "group", id: string, name: string) {
  renameTarget.value = { type, id, currentName: name };
  renameValue.value = name;
  menuOpen.value = "";
}

async function handleRename() {
  if (!renameTarget.value || !renameValue.value.trim()) return;
  try {
    if (renameTarget.value.type === "file") {
      await renameFile(renameTarget.value.id, renameValue.value.trim(), currentUserId());
    } else {
      await renameFileGroup(renameTarget.value.id, renameValue.value.trim(), currentUserId());
    }
    renameTarget.value = null;
    await loadFileList();
  } catch (err) {
    statusText.value = `改名失败：${(err as Error).message}`;
  }
}

async function handleTogglePin(type: "file" | "group", id: string) {
  try {
    if (type === "file") await togglePinFile(id, currentUserId());
    else await togglePinFileGroup(id, currentUserId());
    menuOpen.value = "";
    await loadFileList();
  } catch (err) {
    statusText.value = `操作失败：${(err as Error).message}`;
  }
}

async function handleAddFileToGroup(fileId: string, groupId: string) {
  const file = files.value.find((item) => item.id === fileId);
  const group = fileGroups.value.find((item) => item.id === groupId);
  if (file?.file_group_id === groupId) {
    addFileToGroupTarget.value = "";
    menuOpen.value = "";
    statusText.value = `「${file.name}」已在该文件组中`;
    return;
  }
  try {
    await addFileToGroup(fileId, groupId, currentUserId());
    addFileToGroupTarget.value = "";
    menuOpen.value = "";
    await loadFileList();
    statusText.value = `已将「${file?.name ?? fileId}」移动到「${group?.name ?? groupId}」`;
  } catch (err) {
    statusText.value = `移动文件失败：${(err as Error).message}`;
  }
}

async function selectFile(id: string) {
  selectedFileId.value = id;
  selectedFileGroupId.value = "";
  isSwitching.value = true;
  try {
    await Promise.all([fetchGraphByFile(id), loadConversation(id, "")]);
  } catch (err) {
    statusText.value = `加载失败：${(err as Error).message}`;
  } finally {
    isSwitching.value = false;
  }
}

async function selectFileGroup(id: string) {
  selectedFileGroupId.value = id;
  selectedFileId.value = "";
  isSwitching.value = true;
  try {
    await Promise.all([fetchGraphByGroup(id), loadConversation("", id)]);
  } catch (err) {
    statusText.value = `加载失败：${(err as Error).message}`;
  } finally {
    isSwitching.value = false;
  }
}

async function fetchGraphByFile(id: string) {
  const r = await getGraphAll({ file_id: id, user_id: currentUserId() });
  graphRawData = preprocessGraphData(r.nodes, r.edges, {
    minConfidence: 0.6,
    removeSelfLoops: true,
    keepIsolatedNodes: false,
  });
  await renderGraph(graphRawData);
  statusText.value = `图谱：${graphRawData.nodes.length} 节点 / ${graphRawData.edges.length} 连线`;
}

async function fetchGraphByGroup(id: string) {
  const r = await getGraphAll({ file_group_id: id, user_id: currentUserId() });
  graphRawData = preprocessGraphData(r.nodes, r.edges, {
    minConfidence: 0.6,
    removeSelfLoops: true,
    keepIsolatedNodes: false,
  });
  await renderGraph(graphRawData);
  statusText.value = `图谱：${graphRawData.nodes.length} 节点 / ${graphRawData.edges.length} 连线`;
}

async function loadConversation(fileId: string, fileGroupId: string) {
  try {
    const c = await getConversation({
      file_id: fileId || undefined,
      file_group_id: fileGroupId || undefined,
      user_id: currentUserId(),
    });
    currentConversationId.value = c.id;
    chatMessages.value = c.messages.map((m) => ({
      role: m.role as "user" | "ai",
      content: m.content,
    }));
  } catch {
    chatMessages.value = [];
    currentConversationId.value = "";
  }
}

async function handleClearConversation() {
  if (!currentConversationId.value) return;
  try {
    await deleteConversation(currentConversationId.value, currentUserId());
    chatMessages.value = [];
    currentConversationId.value = "";
  } catch (err) {
    statusText.value = `清空对话失败：${(err as Error).message}`;
  }
}

async function sendChatMessage(imageBase64 = "") {
  const msg = chatInput.value.trim();
  if ((!msg && !imageBase64) || isChatting.value) return;
  chatMessages.value.push({ role: "user", content: msg || "[图片]" });
  chatInput.value = "";
  isChatting.value = true;
  if (!currentConversationId.value) {
    await loadConversation(selectedFileId.value, selectedFileGroupId.value);
  }
  saveMessage({
    conversation_id: currentConversationId.value,
    file_id: selectedFileId.value || undefined,
    file_group_id: selectedFileGroupId.value || undefined,
    role: "user",
    content: msg || "[图片]",
  }).catch(() => {});
  try {
    const gn = JSON.stringify(
      graphRawData.nodes.map((n) => ({
        name: n.label || n.id,
        status: n.status,
        reason: n.reason,
        definition: n.data?.definition ?? "",
      })),
    );
    const ge = JSON.stringify(
      graphRawData.edges.map((e) => ({
        source: e.source,
        target: e.target,
        relation: e.label,
        status: e.status,
        reason: e.reason,
      })),
    );
    const r = await chatWithContext({
      user_message: msg || "请分析这张图片",
      conversation_id: currentConversationId.value,
      markdown: markdown.value,
      graph_nodes: gn,
      graph_edges: ge,
      image_base64: imageBase64,
    });
    chatMessages.value.push({ role: "ai", content: r.reply });
    saveMessage({
      conversation_id: currentConversationId.value,
      file_id: selectedFileId.value || undefined,
      file_group_id: selectedFileGroupId.value || undefined,
      role: "ai",
      content: r.reply,
    }).catch(() => {});
    if (r.edited_markdown) {
      markdown.value = r.edited_markdown;
      statusText.value = "AI 已修改文档，可点击生成图谱查看更新";
    }
  } catch (err) {
    chatMessages.value.push({ role: "ai", content: `对话出错：${(err as Error).message}` });
  } finally {
    isChatting.value = false;
  }
}

function handleChatImageUpload(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input?.files?.[0];
  if (!file) return;
  const reader = new FileReader();
  reader.onload = () => {
    const b64 = (reader.result as string).split(",")[1];
    void sendChatMessage(b64);
  };
  reader.readAsDataURL(file);
  input.value = "";
}

async function showNodeDetail(nodeId: string) {
  const node = graphRawData.nodes.find((n) => n.id === nodeId);
  if (!node) return;
  const snippets = extractMarkdownSnippets(node.label || node.id, markdown.value);
  selectedNodeDetail.value = {
    id: node.id,
    label: node.label || node.id,
    type: node.type,
    status: node.status,
    reason: node.reason,
    snippets,
    aiExplanation: buildAIExplanation(
      { label: node.label || node.id, status: node.status, reason: node.reason },
      snippets,
    ),
  };
  if (!markdown.value.trim()) return;
  isExplaining.value = true;
  try {
    const r = await explainConcept({
      concept: node.label || node.id,
      markdown: markdown.value,
      user_id: currentUserId(),
    });
    if (selectedNodeDetail.value?.id === node.id) {
      selectedNodeDetail.value.aiExplanation = r.explanation;
    }
  } catch (err) {
    if (selectedNodeDetail.value?.id === node.id) {
      selectedNodeDetail.value.aiExplanation = `讲解失败：${(err as Error).message}`;
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
    node: getNodeConfig(),
    edge: getEdgeConfig(),
    layout: getLayoutConfig("force"),
    behaviors: ["drag-canvas", "zoom-canvas", "drag-element"],
    plugins: [
      {
        type: "tooltip",
        trigger: "hover",
        enable: (e: any) => {
          if (e.targetType !== "node") return false;
          const s = String(e.target?.data?.status ?? "");
          return s === "error" || s === "supplement";
        },
        getContent: (e: any) => tooltipHtml(String(e.target?.data?.reason ?? "")),
      },
    ],
  });
  await graph.render();
  graph.on("node:click", (e: any) => {
    const id = resolveNodeIdFromEvent(e);
    if (!id) return;
    if (isLightRAGMode.value && !expandedNodes.value.has(id)) {
      void expandNodeNeighbors(id);
    } else if (isFocusMode.value) {
      void focusNode(id);
      void showNodeDetail(id);
    } else {
      void showNodeDetail(id);
    }
  });
  graph.on("node:pointerenter", (e: any) => {
    const id = resolveNodeIdFromEvent(e);
    if (!id || !graph) return;
    const edgeIds = graphRawData.edges
      .filter((edge) => edge.source === id || edge.target === id)
      .map((edge) => edge.id || `${edge.source}-${edge.target}-${edge.label}`);
    graph.setElementState(id, "highlight");
    edgeIds.forEach((edgeId) => graph?.setElementState(edgeId, "highlight"));
  });
  graph.on("node:pointerleave", () => {
    graph?.getData().nodes.forEach((node: any) => graph?.setElementState(node.id, []));
    graph?.getData().edges.forEach((edge: any) => graph?.setElementState(edge.id, []));
  });
  graph.on("canvas:click", () => {
    selectedNodeDetail.value = null;
  });
}

function graphDataForDisplay(data: GraphResponse): GraphResponse {
  if (graphStatusFilter.value === "all") return data;
  const nodes = data.nodes.filter((node) => node.status === graphStatusFilter.value);
  const ids = new Set(nodes.map((node) => node.id));
  return { nodes, edges: data.edges.filter((edge) => ids.has(edge.source) && ids.has(edge.target)) };
}

function localFocusData(nodeId: string, depth = 2): GraphResponse {
  const nodeIds = new Set([nodeId]);
  for (let level = 0; level < depth; level++) {
    graphRawData.edges.forEach((edge) => {
      if (nodeIds.has(edge.source) || nodeIds.has(edge.target)) {
        nodeIds.add(edge.source);
        nodeIds.add(edge.target);
      }
    });
  }
  return {
    nodes: graphRawData.nodes.filter((node) => nodeIds.has(node.id)),
    edges: graphRawData.edges.filter((edge) => nodeIds.has(edge.source) && nodeIds.has(edge.target)),
  };
}

function preferredLayout(): LayoutType {
  if (graphLayoutMode.value !== "auto") return graphLayoutMode.value;
  return isNavigating.value || Boolean(selectedFileId.value) ? "dagre" : "force";
}

function prepareHierarchyData(data: GraphResponse): GraphResponse {
  const dependencyPattern = /PREREQUISITE|DEPENDS|REQUIRES|PRECEDES|PARENT|IS_A|PART_OF/i;
  const hierarchyEdges = data.edges.filter((edge) => dependencyPattern.test(edge.label || ""));
  if (hierarchyEdges.length === 0) return data;

  const supportingEdges = data.edges.filter((edge) =>
    edge.status === "error" || edge.status === "supplement" || dependencyPattern.test(edge.label || ""),
  );
  const connected = new Set<string>();
  supportingEdges.forEach((edge) => {
    connected.add(edge.source);
    connected.add(edge.target);
  });
  return {
    nodes: data.nodes.filter((node) => connected.has(node.id)),
    edges: supportingEdges,
  };
}

async function renderGraph(
  data: GraphResponse,
  focusMode = false,
  pathData: GraphResponse[] = [],
  layoutType: LayoutType = "force",
) {
  if (!graph) return;
  const selectedLayout = graphLayoutMode.value === "auto" ? layoutType : graphLayoutMode.value;
  activeLayout.value = selectedLayout;
  const visibleData = graphDataForDisplay(selectedLayout === "dagre" ? prepareHierarchyData(data) : data);
  const focusSet = focusMode ? buildFocusSet(pathData) : undefined;
  const styled = buildStyledGraph(visibleData, focusSet);
  graph.setLayout(
    styled.nodes.some((n: any) => n.x !== undefined)
      ? { type: "preset", padding: 50 }
      : getLayoutConfig(selectedLayout),
  );
  graph.setData(styled as any);
  await graph.render();
}

async function expandNodeNeighbors(nodeId: string) {
  if (!graph) return;
  statusText.value = `展开「${nodeId}」的邻居…`;
  try {
    const nb = await getNodeNeighbors(nodeId, currentUserId(), 1);
    const existN = new Set(graphRawData.nodes.map((n) => n.id));
    const existE = new Set(graphRawData.edges.map((e) => e.id));
    const nn = nb.nodes.filter((n) => !existN.has(n.id));
    const ne = nb.edges.filter((e) => !existE.has(e.id));
    if (nn.length === 0 && ne.length === 0) {
      statusText.value = `「${nodeId}」无更多邻居`;
      expandedNodes.value.add(nodeId);
      return;
    }
    graphRawData.nodes.push(...nn);
    graphRawData.edges.push(...ne);
    expandedNodes.value.add(nodeId);
    const s = buildStyledGraph({ nodes: nn, edges: ne });
    graph.addData(s as any);
    await graph.render();
    statusText.value = `已展开 ${nn.length} 节点 ${ne.length} 边`;
  } catch (e) {
    statusText.value = `展开失败：${(e as Error).message}`;
  }
}

function extractCoreNodes(data: GraphResponse, n: number): GraphResponse {
  const deg = new Map<string, number>();
  data.nodes.forEach((x) => deg.set(x.id, 0));
  data.edges.forEach((e) => {
    deg.set(e.source, (deg.get(e.source) || 0) + 1);
    deg.set(e.target, (deg.get(e.target) || 0) + 1);
  });
  const sn = [...data.nodes]
    .sort((a, b) => (deg.get(b.id) || 0) - (deg.get(a.id) || 0))
    .slice(0, n);
  const ids = new Set(sn.map((x) => x.id));
  return {
    nodes: sn,
    edges: data.edges.filter((e) => ids.has(e.source) && ids.has(e.target)),
  };
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
  const raw = await getGraphAll({ user_id: currentUserId() });
  graphRawData = preprocessGraphData(raw.nodes, raw.edges, {
    minConfidence: 0.6,
    removeSelfLoops: true,
    keepIsolatedNodes: false,
  });
  if (isLightRAGMode.value) {
    graphRawData = extractCoreNodes(graphRawData, 5);
    expandedNodes.value.clear();
  }
  await renderGraph(graphRawData);
  statusText.value = `图谱：${graphRawData.nodes.length} 节点 / ${graphRawData.edges.length} 连线`;
}

function pushDiagnosisSummary(result: Record<string, any>) {
  const diag = result.diagnose_result as Record<string, any> | undefined;
  const nodes: Array<Record<string, any>> = diag?.nodes ?? [];
  const errors = nodes.filter((n) => n.status === "error");
  const supplements = nodes.filter((n) => n.status === "supplement");
  const corrects = nodes.filter((n) => n.status === "correct");

  let summary = `## 笔记诊断报告\n\n`;
  summary += `共识别 **${nodes.length}** 个知识点，生成 **${result.relations_count ?? 0}** 条关系。\n\n`;

  if (errors.length > 0) {
    summary += `### 发现 ${errors.length} 处错误\n`;
    errors.forEach((n) => {
      summary += `- **${n.name}**：${n.reason || "笔记描述有误"}\n`;
    });
    summary += "\n";
  }
  if (supplements.length > 0) {
    summary += `### AI 补全 ${supplements.length} 处知识缺口\n`;
    supplements.forEach((n) => {
      summary += `- **${n.name}**：${n.reason || "该前置知识在笔记中缺失"}\n`;
    });
    summary += "\n";
  }
  if (corrects.length > 0) {
    summary += `### 正确的知识点（${corrects.length} 个）\n`;
    summary += corrects
      .slice(0, 5)
      .map((n) => `- ${n.name}`)
      .join("\n");
    if (corrects.length > 5) summary += `\n... 等 ${corrects.length - 5} 个`;
    summary += "\n\n";
  }
  if (errors.length === 0 && supplements.length === 0) {
    summary += `笔记质量很好，未发现错误或知识缺口。\n\n`;
  }
  summary += `你可以继续提问，让我帮你修正错误或补充缺失知识点。`;

  chatMessages.value.push({ role: "ai", content: summary });
}

async function handleUpload() {
  if (!canGenerate.value) return;
  isLoading.value = true;
  statusText.value = "AI 诊断中（NER → 校验 → 补全）…";
  try {
    const r = await uploadNoteLangChain({
      markdown: markdown.value,
      user_id: currentUserId(),
      file_id: selectedFileId.value || undefined,
      file_group_id: selectedFileGroupId.value || undefined,
    });
    if (r.file_id && !selectedFileId.value) selectedFileId.value = r.file_id as string;
    await loadFileList();
    await fetchAllGraph();
    pushDiagnosisSummary(r);
    isNavigating.value = false;
    selectedNodeDetail.value = null;
  } catch (err) {
    statusText.value = `生成失败：${(err as Error).message}`;
  } finally {
    isLoading.value = false;
  }
}

async function handleImportMarkdownFile(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input?.files?.[0];
  if (!file) return;
  if (!file.name.endsWith(".md") && file.type !== "text/markdown") {
    statusText.value = "仅支持 .md 文件";
    input.value = "";
    return;
  }
  if (file.size > 2 * 1024 * 1024) {
    statusText.value = "文件过大（>2MB）";
    input.value = "";
    return;
  }
  try {
    markdown.value = await file.text();
    importedFileName.value = file.name;
    statusText.value = `已导入：${file.name}`;
    input.value = "";
  } catch (err) {
    statusText.value = `读取失败：${(err as Error).message}`;
    input.value = "";
  }
}

async function handleLeftUploadFile(event: Event) {
  await handleImportMarkdownFile(event);
  if (markdown.value) await handleUpload();
}

async function handleLeftUploadFileGroup(event: Event) {
  const input = event.target as HTMLInputElement;
  const fls = input?.files;
  if (!fls || fls.length === 0) return;
  if (!requireLogin()) {
    input.value = "";
    return;
  }
  isLoading.value = true;
  statusText.value = `创建文件组并处理 ${fls.length} 个文件…`;
  try {
    const gn = `文件组_${new Date().toLocaleDateString()}`;
    const gr = await createFileGroup(gn, currentUserId());
    const gid = (gr as any).group_id as string;
    for (const f of Array.from(fls)) {
      statusText.value = `处理：${f.name}…`;
      const c = await f.text();
      await uploadNoteLangChain({
        markdown: c,
        user_id: currentUserId(),
        file_group_id: gid,
      });
    }
    await loadFileList();
    selectedFileGroupId.value = gid;
    await fetchGraphByGroup(gid);
    statusText.value = `文件组已创建：${fls.length} 个文件`;
  } catch (err) {
    statusText.value = `创建文件组失败：${(err as Error).message}`;
  } finally {
    isLoading.value = false;
    input.value = "";
  }
}

async function handlePathNavigate() {
  if (!canNavigate.value) return;
  isNavigating.value = true;
  statusText.value = "计算逆向学习路径…";
  try {
    const r = await getGraphPath(concept.value.trim(), currentUserId(), maxDepth.value);
    const hasRelated = r.all_related && r.all_related.nodes && r.all_related.nodes.length > 0;
    await renderGraph(
      hasRelated ? r.all_related! : graphRawData,
      true,
      hasRelated ? [r.all_related!] : r.paths,
      "dagre",
    );
    if (r.dependency_tree?.length) {
      const g = await getLearningPath({
        target_concept: concept.value.trim(),
        dependency_tree_json: JSON.stringify(r.dependency_tree),
        graph_nodes_json: JSON.stringify(r.all_related?.nodes ?? graphRawData.nodes),
      }).catch(() => null);
      if (g?.guidance) {
        chatMessages.value.push({ role: "ai", content: g.guidance });
      }
    }
    statusText.value = `专注模式：${r.paths.length} 条路径，${r.dependency_tree?.length ?? 0} 个前置节点`;
  } catch (err) {
    statusText.value = `路径查询失败：${(err as Error).message}`;
  }
}

async function resetFocus() {
  isFocusMode.value = false;
  focusedNodeId.value = "";
  await renderGraph(graphRawData, false, [], "force");
  isNavigating.value = false;
  statusText.value = "已退出专注模式";
}

async function focusNode(nodeId: string) {
  const focused = localFocusData(nodeId);
  isFocusMode.value = true;
  focusedNodeId.value = nodeId;
  await renderGraph(graphRawData, true, [focused], "force");
  await (graph as any)?.focusElement?.(nodeId, { animation: { duration: 500 } });
  statusText.value = `专注模式：${focused.nodes.length} 个相关知识点`;
}

async function searchGraph() {
  const keyword = graphSearch.value.trim().toLowerCase();
  if (!keyword) return;
  const node = graphRawData.nodes.find((item) => (item.label || item.id).toLowerCase().includes(keyword));
  if (!node) {
    statusText.value = `未找到概念「${graphSearch.value.trim()}」`;
    return;
  }
  await focusNode(node.id);
  await showNodeDetail(node.id);
}

async function applyStatusFilter() {
  await renderGraph(graphRawData, isFocusMode.value, isFocusMode.value ? [localFocusData(focusedNodeId.value)] : [], preferredLayout());
  statusText.value = graphStatusFilter.value === "all" ? "已显示全部状态" : `已筛选：${graphStatusFilter.value}`;
}

async function applyLayoutMode() {
  await renderGraph(
    graphRawData,
    isFocusMode.value,
    isFocusMode.value && focusedNodeId.value ? [localFocusData(focusedNodeId.value)] : [],
    preferredLayout(),
  );
  statusText.value = activeLayout.value === "dagre" ? "层级布局：前置知识从左向右排列" : "关系网络：按关联强度排列";
}

function fitGraph() {
  void (graph as any)?.fitView?.({ when: "always", direction: "both" });
}

function zoomGraph(ratio: number) {
  void (graph as any)?.zoomBy?.(ratio);
}

onMounted(async () => {
  await initGraph();
  await loadFileList({ silent: true });
  try {
    await fetchAllGraph();
  } catch {
    statusText.value = "图谱待生成";
  }
  if (graphRoot.value && graph) {
    resizeObserver = new ResizeObserver((e) => {
      const r = e[0];
      if (r && graph) graph.resize(r.contentRect.width, r.contentRect.height);
    });
    resizeObserver.observe(graphRoot.value);
  }
});

onBeforeUnmount(() => {
  window.removeEventListener("mousemove", onDividerMousemove);
  window.removeEventListener("mouseup", onDividerMouseup);
  resizeObserver?.disconnect();
  graph?.destroy();
  graph = null;
});
</script>

<template>
  <main class="flex h-screen w-full overflow-hidden bg-[#F8FAFC]">
    <!-- 左侧：文件管理（可拖拽调宽） -->
    <FileSidebar
      v-model:new-group-name="newGroupName"
      v-model:menu-open="menuOpen"
      :width="leftWidth"
      :logged-in-user-id="loggedInUserId"
      :files="files"
      :file-groups="fileGroups"
      :selected-file-id="selectedFileId"
      :selected-file-group-id="selectedFileGroupId"
      :is-refreshing="isRefreshing"
      @login-required="showLoginPrompt = true"
      @upload-file="handleLeftUploadFile"
      @upload-file-group="handleLeftUploadFileGroup"
      @create-group="handleCreateGroup"
      @select-file="selectFile"
      @select-file-group="selectFileGroup"
      @toggle-pin="handleTogglePin"
      @rename="openRenameDialog"
      @delete-file="handleDeleteFile"
      @delete-group="handleDeleteGroup"
      @add-to-group="(id) => (addFileToGroupTarget = id)"
      @refresh="handleRefresh"
    />

    <div
      class="group relative w-1.5 shrink-0 cursor-col-resize bg-transparent transition-colors hover:bg-indigo-300 active:bg-indigo-400"
      @mousedown="onDividerMousedown('left', $event)"
    >
      <div class="absolute inset-y-0 -left-1 -right-1" />
      <div
        class="absolute left-1/2 top-1/2 h-8 w-1 -translate-x-1/2 -translate-y-1/2 rounded-full bg-slate-300 transition-colors group-hover:bg-indigo-400"
      />
    </div>

    <!-- 中间：知识图谱（视觉中心，占满剩余宽度） -->
    <section class="flex min-w-0 flex-1 flex-col overflow-hidden">
      <header class="flex items-center gap-3 border-b border-gray-200 bg-white px-5 py-3">
        <div class="min-w-0">
          <h1 class="text-lg font-semibold text-slate-900">Learning Graph</h1>
          <p class="truncate text-[13px] text-slate-500">
            {{ isSwitching ? "加载中…" : statusText }}
          </p>
        </div>
        <div class="flex-1" />
        <button
          type="button"
          class="h-11 shrink-0 rounded-xl border border-gray-200 bg-white px-4 text-sm font-medium text-slate-700 transition duration-200 hover:bg-gray-100"
          @click="isLightRAGMode = !isLightRAGMode; fetchAllGraph()"
        >
          {{ isLightRAGMode ? "渐进式展开" : "显示全图" }}
        </button>
      </header>

      <div class="relative m-3 min-h-0 flex-1 overflow-hidden rounded-[18px] border border-gray-200 bg-white shadow-sm">
        <div ref="graphRoot" class="absolute inset-0" />

        <div class="absolute left-4 top-4 z-20 flex flex-wrap items-center gap-2 rounded-2xl border border-slate-200 bg-white/95 p-2 shadow-sm backdrop-blur">
          <div class="flex h-9 items-center overflow-hidden rounded-xl border border-slate-200 bg-white">
            <input v-model="graphSearch" class="w-40 px-3 text-sm outline-none" placeholder="搜索概念…" @keyup.enter="searchGraph" />
            <button class="h-full border-l border-slate-200 px-3 text-xs font-medium text-indigo-600 hover:bg-indigo-50" @click="searchGraph">定位</button>
          </div>
          <select v-model="graphStatusFilter" class="h-9 rounded-xl border border-slate-200 bg-white px-2 text-xs text-slate-600 outline-none" @change="applyStatusFilter">
            <option value="all">全部状态</option>
            <option value="correct">正确</option>
            <option value="error">错误</option>
            <option value="supplement">AI 补全</option>
          </select>
          <select v-model="graphLayoutMode" class="h-9 rounded-xl border border-slate-200 bg-white px-2 text-xs text-slate-600 outline-none" @change="applyLayoutMode">
            <option value="auto">自动布局</option>
            <option value="dagre">学习层级</option>
            <option value="force">关系网络</option>
          </select>
          <button class="h-9 rounded-xl px-3 text-xs font-medium transition" :class="isFocusMode ? 'bg-indigo-600 text-white' : 'bg-slate-100 text-slate-600 hover:bg-slate-200'" @click="isFocusMode ? resetFocus() : (isFocusMode = true)">
            {{ isFocusMode ? "退出专注" : "专注模式" }}
          </button>
          <button class="h-9 rounded-xl bg-slate-100 px-3 text-xs text-slate-600 hover:bg-slate-200" @click="fitGraph">适应屏幕</button>
          <button class="h-9 w-9 rounded-xl bg-slate-100 text-sm text-slate-600 hover:bg-slate-200" @click="zoomGraph(1.2)">＋</button>
          <button class="h-9 w-9 rounded-xl bg-slate-100 text-sm text-slate-600 hover:bg-slate-200" @click="zoomGraph(0.8)">−</button>
        </div>

        <div class="absolute bottom-4 right-4 z-10 rounded-xl border border-slate-200 bg-white/90 px-3 py-2 text-[11px] text-slate-500 shadow-sm backdrop-blur">
          <span class="mr-3"><i class="mr-1 inline-block h-2.5 w-2.5 rounded-full bg-blue-500" />正确</span>
          <span class="mr-3"><i class="mr-1 inline-block h-2.5 w-2.5 rounded-full bg-red-500" />错误</span>
          <span><i class="mr-1 inline-block h-2.5 w-2.5 rounded-full border border-dashed border-violet-600 bg-violet-100" />AI 补全</span>
        </div>

        <div
          v-if="selectedNodeDetail"
          class="absolute bottom-4 left-4 right-4 max-w-md rounded-[18px] border border-gray-200 bg-white/95 p-4 shadow-md backdrop-blur"
        >
          <div class="mb-2 flex items-start justify-between gap-3">
            <div>
              <p class="text-base font-semibold text-slate-900">{{ selectedNodeDetail.label }}</p>
              <span
                class="mt-1 inline-block rounded-lg px-2 py-0.5 text-[13px] font-medium"
                :class="
                  selectedNodeDetail.status === 'error'
                    ? 'bg-red-50 text-red-600'
                    : selectedNodeDetail.status === 'supplement'
                      ? 'bg-violet-50 text-violet-700'
                      : 'bg-slate-100 text-slate-600'
                "
              >
                {{ selectedNodeDetail.status || "unknown" }}
              </span>
            </div>
            <button
              type="button"
              class="text-slate-400 transition hover:text-slate-600"
              @click="selectedNodeDetail = null"
            >
              ×
            </button>
          </div>
          <p v-if="selectedNodeDetail.reason" class="mb-2 text-[13px] text-indigo-600">
            {{ selectedNodeDetail.reason }}
          </p>
          <p v-if="isExplaining" class="text-[13px] text-slate-400">讲解生成中…</p>
          <p v-else class="whitespace-pre-wrap text-sm leading-relaxed text-slate-600">
            {{ selectedNodeDetail.aiExplanation }}
          </p>
        </div>
      </div>
    </section>

    <div
      class="group relative w-1.5 shrink-0 cursor-col-resize bg-transparent transition-colors hover:bg-violet-300 active:bg-violet-400"
      @mousedown="onDividerMousedown('right', $event)"
    >
      <div class="absolute inset-y-0 -left-1 -right-1" />
      <div
        class="absolute left-1/2 top-1/2 h-8 w-1 -translate-x-1/2 -translate-y-1/2 rounded-full bg-slate-300 transition-colors group-hover:bg-violet-400"
      />
    </div>

    <!-- 右侧：导入 / 导航 / 对话（可拖拽调宽） -->
    <aside
      :style="{ width: `${rightWidth}px` }"
      class="flex shrink-0 flex-col gap-3 overflow-hidden border-l border-gray-200 bg-[#F8FAFC] p-3"
    >
      <ImportPanel
        v-model:markdown="markdown"
        v-model:user-id="userId"
        :imported-file-name="importedFileName"
        :logged-in-user-id="loggedInUserId"
        :is-loading="isLoading"
        :can-generate="canGenerate"
        @login="handleLogin"
        @import-file="handleImportMarkdownFile"
        @generate="handleUpload"
      />

      <LearningNavPanel
        v-model:concept="concept"
        v-model:max-depth="maxDepth"
        :can-navigate="canNavigate"
        :is-loading="isLoading"
        :is-navigating="isNavigating"
        @navigate="handlePathNavigate"
        @reset="resetFocus"
      />

      <AiChatPanel
        v-model:chat-input="chatInput"
        :messages="chatMessages"
        :is-chatting="isChatting"
        :has-conversation="!!currentConversationId"
        :render-markdown="renderMarkdown"
        @send="sendChatMessage()"
        @clear="handleClearConversation"
        @upload-image="handleChatImageUpload"
      />
    </aside>

    <div
      v-if="showLoginPrompt"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/20"
      @click.self="showLoginPrompt = false"
    >
      <div class="w-[320px] rounded-[20px] border border-gray-200 bg-white p-6 text-center shadow-lg">
        <p class="text-base font-semibold text-slate-800">需要先登录</p>
        <p class="mt-2 text-sm text-slate-500">文件存储需要输入 user_id 并登录。右侧笔记区可直接使用。</p>
        <button
          type="button"
          class="mt-4 h-11 w-full rounded-xl bg-indigo-600 text-sm font-medium text-white transition hover:bg-indigo-500"
          @click="showLoginPrompt = false"
        >
          知道了
        </button>
      </div>
    </div>

    <div
      v-if="addFileToGroupTarget"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/20"
      @click.self="addFileToGroupTarget = ''"
    >
      <div class="w-[280px] rounded-[20px] border border-gray-200 bg-white p-4 shadow-lg">
        <p class="mb-3 text-sm font-semibold text-slate-800">选择目标文件组</p>
        <div class="mb-3 max-h-[180px] space-y-1 overflow-y-auto">
          <button
            v-for="g in fileGroups"
            :key="g.id"
            type="button"
            :disabled="files.find((f) => f.id === addFileToGroupTarget)?.file_group_id === g.id"
            class="flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm text-slate-700 hover:bg-violet-50 disabled:cursor-default disabled:bg-slate-50 disabled:text-slate-400"
            @click="handleAddFileToGroup(addFileToGroupTarget, g.id)"
          >
            <span class="truncate">{{ g.name }}</span>
            <span v-if="files.find((f) => f.id === addFileToGroupTarget)?.file_group_id === g.id" class="text-[12px]">当前</span>
          </button>
          <p v-if="fileGroups.length === 0" class="px-2 text-sm text-slate-400">暂无文件组</p>
        </div>
        <button
          type="button"
          class="h-11 w-full rounded-xl bg-slate-100 text-sm text-slate-600 hover:bg-slate-200"
          @click="addFileToGroupTarget = ''"
        >
          取消
        </button>
      </div>
    </div>

    <div
      v-if="renameTarget"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/20"
      @click.self="renameTarget = null"
    >
      <div class="w-[280px] rounded-[20px] border border-gray-200 bg-white p-4 shadow-lg">
        <p class="mb-3 text-sm font-semibold text-slate-800">
          {{ renameTarget.type === "file" ? "重命名文件" : "重命名文件组" }}
        </p>
        <input
          v-model="renameValue"
          class="mb-3 h-11 w-full rounded-xl border border-gray-200 px-3 text-sm outline-none focus:border-indigo-500"
          @keyup.enter="handleRename"
        />
        <div class="flex gap-2">
          <button
            type="button"
            class="h-11 flex-1 rounded-xl bg-slate-100 text-sm text-slate-600 hover:bg-slate-200"
            @click="renameTarget = null"
          >
            取消
          </button>
          <button
            type="button"
            class="h-11 flex-1 rounded-xl bg-indigo-600 text-sm text-white hover:bg-indigo-500"
            @click="handleRename"
          >
            确认
          </button>
        </div>
      </div>
    </div>

    <div
      v-if="isLoading"
      class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/30 backdrop-blur-[2px]"
    >
      <div class="rounded-[20px] border border-white/10 bg-slate-900/90 px-8 py-6 text-center text-white shadow-lg">
        <div class="mx-auto h-10 w-10 animate-spin rounded-full border-2 border-white/20 border-t-white" />
        <p class="mt-3 text-sm">AI 正在生成知识图谱…</p>
      </div>
    </div>
  </main>
</template>
