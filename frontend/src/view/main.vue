<script setup lang="ts">
import { Graph } from "@antv/g6";
import { computed, nextTick, onBeforeUnmount, onMounted, onUnmounted, ref, watch } from "vue";
import { marked } from "marked";
import katex from "katex";
import "katex/dist/katex.min.css";
import {
  addFileToGroup,
  chatWithContext,
  createFileGroup,
  deleteConversation,
  deleteFile,
  deleteFileGroup,
  explainConcept,
  extractToMarkdown,
  getConversation,
  getFilesMarkdown,
  getGraphAll,
  getGraphPath,
  getLearningPath,
  getNodeNeighbors,
  listUserFiles,
  renameFile,
  renameFileGroup,
  saveMessage,
  syncFileToWorkspace,
  togglePinFile,
  togglePinFileGroup,
  uploadNoteLangChain,
  updateFileContent,
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
import WorkspacePanel from "../components/WorkspacePanel.vue";
import ToastStack, { type ToastItem } from "../components/ToastStack.vue";
import SidebarRail, { type RailItem } from "../components/SidebarRail.vue";
import { PanelRightClose, FolderTree, FileUp, Compass, MessageSquare, FolderOpen, RefreshCw } from "lucide-vue-next";

/**
 * 渲染 LaTeX 片段为 HTML。
 * 失败时回退为原始文本，确保公式语法错误不会让整条消息渲染不出来。
 */
function renderLatex(tex: string, displayMode: boolean): string {
  try {
    return katex.renderToString(tex, {
      displayMode,
      throwOnError: false,
      strict: false,
      output: "html",
    });
  } catch {
    return `<code>${tex.replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" }[c] as string))}</code>`;
  }
}

/**
 * 渲染 Markdown 并支持 LaTeX 数学公式。
 *
 * 顺序很关键：必须先抽出数学片段并占位，再做 Markdown 解析，
 * 否则公式里的 `_`、`*`、`\\` 会被 markdown 当成强调/转义符，导致公式损坏。
 * 解析完成后再把占位符替换回 KaTeX 渲染结果。
 *
 * 支持：$$...$$（块级）、\[...\]（块级）、$...$（行内）、\(...\)（行内）。
 */
function renderMarkdown(text: string): string {
  if (!text) return "";

  const mathFragments: string[] = [];

  // 占位符使用不含 markdown 特殊字符的标记
  const stash = (tex: string, displayMode: boolean) => {
    const idx = mathFragments.length;
    mathFragments.push(renderLatex(tex, displayMode));
    return `\u0000MATH${idx}\u0000`;
  };

  let src = text;

  // 1) 块级公式：$$...$$ 与 \[...\]
  src = src.replace(/\$\$([\s\S]+?)\$\$/g, (_m, tex) => stash(tex.trim(), true));
  src = src.replace(/\\\[([\s\S]+?)\\\]/g, (_m, tex) => stash(tex.trim(), true));

  // 2) 行内公式：$...$ 与 \(...\)
  //    排除 $$ 的情况，且要求 $ 前后不是数字，避免误伤「$5 到 $10」这类金额
  src = src.replace(/(?<!\$)\$(?!\s)([^\n$]+?)(?<!\s)\$(?!\$)/g, (m, tex, offset, whole) => {
    const prev = offset > 0 ? whole[offset - 1] : "";
    const next = whole[offset + m.length] ?? "";
    if (/[0-9]/.test(prev) || /[0-9]/.test(next)) return m;
    return stash(tex.trim(), false);
  });
  src = src.replace(/\\\(([\s\S]+?)\\\)/g, (_m, tex) => stash(tex.trim(), false));

  // 3) Markdown 解析
  let html = marked.parse(src, { breaks: true }) as string;

  // 4) 还原公式。Markdown 可能把 \u0000MATH0\u0000 包进 <p> 里，直接字符串替换即可
  html = html.replace(/\u0000MATH(\d+)\u0000/g, (_m, i) => mathFragments[Number(i)] ?? "");

  return html;
}

const markdown = ref("");
const concept = ref("");
const userId = ref("");
const LOGGED_IN_USER_STORAGE_KEY = "learning_graph_user_id";
const loggedInUserId = ref(localStorage.getItem(LOGGED_IN_USER_STORAGE_KEY) || "");
const maxDepth = ref(3);

// 深度上限，必须与后端 repository.graph_repository.go 里的
// maxPathDepthLimit（当前为 6）保持一致。
//
// 这里原先是 12，但后端硬上限是 6：用户把深度拉到 7~12 时后端会静默按 6 算，
// 界面上却毫无提示（因为后端当时也没有回传 meta）。现在两边对齐为 6，
// 用户根本选不到会被截断的值；同时后端已回传 meta，双保险。
const MAX_PATH_DEPTH_LIMIT = 6;
const isLoading = ref(false);
const isNavigating = ref(false);
const statusText = ref("图谱待生成");
const importedFileName = ref("");
const graphRoot = ref<HTMLDivElement | null>(null);
const graphCanvas = ref<HTMLDivElement | null>(null);
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

// ==================== 侧栏折叠 ====================
// 折叠后侧栏变成一列图标（SidebarRail），保留功能入口。
// 两处触发：手动点折叠按钮、拖拽到小于阈值时自动折叠。
const COLLAPSE_THRESHOLD = 160; // 拖拽窄于此值自动折叠
const COLLAPSED_WIDTH = 56;     // 图标栏宽度（w-14 = 3.5rem = 56px）

const leftCollapsed = ref(false);
const rightCollapsed = ref(false);

// 折叠状态持久化，刷新后保持一致
try {
  leftCollapsed.value = localStorage.getItem("lg.leftCollapsed") === "1";
  rightCollapsed.value = localStorage.getItem("lg.rightCollapsed") === "1";
} catch {
  /* localStorage 不可用时忽略，用默认展开态 */
}
watch(leftCollapsed, (v) => {
  try { localStorage.setItem("lg.leftCollapsed", v ? "1" : "0"); } catch {}
});
watch(rightCollapsed, (v) => {
  try { localStorage.setItem("lg.rightCollapsed", v ? "1" : "0"); } catch {}
});

// 展开时定位到的面板（图标点击后高亮 / 滚动）
// 面板锚点：点击图标展开后滚动到对应面板
const importPanelEl = ref<HTMLElement | null>(null);
const navPanelEl = ref<HTMLElement | null>(null);
const chatPanelEl = ref<HTMLElement | null>(null);
const workspacePanelEl = ref<HTMLElement | null>(null);

// 折叠态图标项：只放功能入口，点击后展开并定位
const leftRailItems = computed<RailItem[]>(() => [
  { key: "upload", label: "上传笔记", icon: FileUp },
  { key: "refresh", label: "刷新文件列表", icon: RefreshCw },
  { key: "groups", label: "文件组", icon: FolderTree },
]);

const rightRailItems = computed<RailItem[]>(() => [
  { key: "import", label: "导入学习笔记", icon: FileUp },
  { key: "nav", label: "逆向学习导航", icon: Compass },
  { key: "chat", label: "AI 学习导师", icon: MessageSquare },
  { key: "workspace", label: "AI 工作区", icon: FolderOpen },
]);

const leftRailUploadInput = ref<HTMLInputElement | null>(null);

const leftActivePanel = ref("");
const rightActivePanel = ref("");

const leftWidth = ref(280);
const rightWidthSaved = ref(340);
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
  const collapseAt = COLLAPSE_THRESHOLD;
  const maxW = 480;

  if (dragging.value === "left") {
    const raw = e.clientX;
    // 拖到很窄时自动折叠，并停止继续变窄（避免出现 10px 的畸形宽度）
    if (raw < collapseAt) {
      leftCollapsed.value = true;
      return;
    }
    leftCollapsed.value = false;
    leftWidth.value = Math.min(maxW, raw);
  } else {
    const raw = window.innerWidth - e.clientX;
    if (raw < collapseAt) {
      rightCollapsed.value = true;
      return;
    }
    rightCollapsed.value = false;
    rightWidth.value = Math.min(maxW, raw);
    rightWidthSaved.value = rightWidth.value;
  }
}

function onDividerMouseup() {
  if (!dragging.value) return;
  dragging.value = null;
  document.body.style.cursor = "";
  document.body.style.userSelect = "";
  resizeGraph();
}

/** 展开某一侧侧栏，并把视图滚动到指定面板 */
async function expandSidebar(side: "left" | "right", panelKey = "") {
  if (side === "left") {
    leftCollapsed.value = false;
    leftActivePanel.value = panelKey;
  } else {
    rightCollapsed.value = false;
    rightWidth.value = rightWidthSaved.value || 340;
    rightActivePanel.value = panelKey;
  }
  await nextTick();
  // 展开后把对应面板滚动到可见位置（右侧内容可能很长）
  if (side === "right" && panelKey) {
    const map: Record<string, HTMLElement | null> = {
      import: importPanelEl.value,
      nav: navPanelEl.value,
      chat: chatPanelEl.value,
      workspace: document.querySelector('[data-panel="workspace"]'),
    };
    map[panelKey]?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  }
  // 侧栏宽度变化后图谱需要重新适配
  resizeGraph();
}

/** 折叠态图标点击：先把侧栏展开，再执行该图标对应的动作 */
async function handleLeftRailSelect(key: string) {
  await expandSidebar("left", key);
  if (key === "upload") {
    leftRailUploadInput.value?.click();
  } else if (key === "refresh") {
    await handleRefresh();
  }
  // groups 仅展开，滚动定位交给 expandSidebar
}

function collapseSidebar(side: "left" | "right") {
  if (side === "left") leftCollapsed.value = true;
  else rightCollapsed.value = true;
  resizeGraph();
}

/**
 * 确保容器尺寸变化能同步到图实例。
 *
 * 必须可重复调用：graph.destroy() 会让 G6 重新创建画布 DOM，
 * 而 ResizeObserver 的观察目标/回调绑定的是旧引用，重建后不再生效，
 * 于是容器变高变宽时画布仍停在旧尺寸 —— 表现为「画布下面一半白屏」。
 * 因此每次重建图之后都要重新挂载一次。
 */
function ensureResizeObserver() {
  resizeObserver?.disconnect();
  if (!graphRoot.value) return;
  resizeObserver = new ResizeObserver((entries) => {
    const r = entries[0];
    if (!r || !graph) return;
    const w = r.contentRect.width;
    const h = r.contentRect.height;
    if (w <= 0 || h <= 0) return;
    // resize 会重置画布尺寸（内容被清掉），因此 resize 之后必须重绘一次，
    // 否则折叠/展开侧栏后画布会变成一片空白。
    graph.resize(w, h);
    void graph.draw();
  });
  resizeObserver.observe(graphRoot.value);
}

function resizeGraph() {
  requestAnimationFrame(() => {
    if (graphRoot.value && graph) {
      const r = graphRoot.value.getBoundingClientRect();
      if (r.width > 0 && r.height > 0) {
        graph.resize(Math.round(r.width), Math.round(r.height));
        // 同 ensureResizeObserver：resize 后需要重绘，否则内容消失
        void graph.draw();
      }
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

/** AI 工作区面板引用（用于在文件变动后刷新其列表） */
const workspacePanelRef = ref<InstanceType<typeof WorkspacePanel> | null>(null);

// ==================== 轻提示（toast） ====================

const toasts = ref<ToastItem[]>([]);
let toastSeq = 0;
const toastTimers = new Map<number, number>();

/**
 * 弹出一条会自动淡出的提示。
 *
 * 用于「查询未命中」「深度被截断」这类只需告知、不需要用户决策的信息；
 * 因此不阻塞操作，到时间自动消失（也允许手动关闭）。
 */
function pushToast(message: string, type: ToastItem["type"] = "info", detail = "", duration = 4200) {
  const id = ++toastSeq;
  toasts.value = [...toasts.value, { id, message, type, detail }];

  const timer = window.setTimeout(() => dismissToast(id), duration);
  toastTimers.set(id, timer);
}

function dismissToast(id: number) {
  const timer = toastTimers.get(id);
  if (timer !== undefined) {
    window.clearTimeout(timer);
    toastTimers.delete(id);
  }
  toasts.value = toasts.value.filter((t) => t.id !== id);
}

onUnmounted(() => {
  toastTimers.forEach((t) => window.clearTimeout(t));
  toastTimers.clear();
});

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
  localStorage.setItem(LOGGED_IN_USER_STORAGE_KEY, u);
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

async function handleLogout() {
  loggedInUserId.value = "";
  localStorage.removeItem(LOGGED_IN_USER_STORAGE_KEY);
  userId.value = "";
  selectedFileId.value = "";
  selectedFileGroupId.value = "";
  chatMessages.value = [];
  currentConversationId.value = "";
  graphRawData = { nodes: [], edges: [] };
  await renderGraph(graphRawData, false, [], "force");
  await loadFileList();
  statusText.value = "已退出登录";
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

/**
 * 把 AI 修改后的正文回写到当前文件。
 *
 * AI 的编辑能力只作用于「文档文本」这一层：原文档在浏览器里，改完若不回写，
 * 刷新后就丢了。这里把结果持久化到该文件节点，使修改真正落地；
 * 全图作用域下没有归属文件，则只保留在编辑框（提示用户先生成图谱）。
 */
async function persistEditedMarkdown(content: string): Promise<void> {
  const targetFileId = selectedFileId.value;
  if (!targetFileId) {
    statusText.value = "AI 已修改文档草稿；保存到文件前请先选择或生成一个文件";
    return;
  }
  try {
    await updateFileContent(targetFileId, content, currentUserId());
    statusText.value = "AI 已修改文档并保存到该文件，可点击生成图谱查看更新";
    // 同步进工作区，这样用户能在「AI 工作区」里直接下载改后的文件
    await handleSyncFileToWorkspace(targetFileId, true);
  } catch (err) {
    statusText.value = `文档已修改，但保存失败：${(err as Error).message}`;
  }
}

/**
 * 把某个已入库文件同步进 AI 工作区，使 AI 能像操作真实文件一样读写它。
 *
 * @param silent 为 true 时不改变状态栏文案（用于自动同步的静默场景）
 */
async function handleSyncFileToWorkspace(fileId: string, silent = false): Promise<void> {
  if (!fileId) return;
  try {
    const r = await syncFileToWorkspace(fileId, currentUserId());
    statusText.value = `已将「${r.path}」同步到 AI 工作区，可在下方下载`;
    if (!silent) {
      pushToast(`已同步「${r.path}」到工作区`, "info", "可在 AI 工作区中预览或下载。");
    }
    await workspacePanelRef.value?.refresh();
  } catch (err) {
    // 同步失败必须给用户可见反馈。
    // 之前只写 statusText，而它显示在图谱顶栏、离工作区很远，
    // 用户点了下拉框却「什么都没发生」——其实请求已发出并被拒绝。
    const raw = (err as Error).message;
    let hint = raw;
    try {
      const parsed = JSON.parse(raw);
      hint = parsed.error || raw;
    } catch {
      // 非 JSON，直接用原文
    }
    statusText.value = `同步到工作区失败：${hint}`;
    if (!silent) {
      // 针对最常见的原因（旧文件没有存正文）给出可执行的建议
      const detail = hint.includes("no content")
        ? "该文件是在「正文存储」功能上线前上传的，云端没有保存它的内容。请重新上传该笔记，之后再同步。"
        : hint;
      pushToast("同步到工作区失败", "error", detail, 8000);
    }
  }
}

async function loadConversation(fileId: string, fileGroupId: string) {  try {
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
  const userContent = msg || "[图片]";
  chatMessages.value.push({ role: "user", content: userContent });
  chatInput.value = "";
  isChatting.value = true;
  try {
    // 需要先确保拿到 conversation_id 才能落库。
    // loadConversation 会用数据库内容整体替换 chatMessages，
    // 因此先记下当前本地消息，替换后再把刚发出的这条补回去，
    // 避免「发送后消息消失、等 AI 回复才出现」的空白现象。
    if (!currentConversationId.value) {
      const localPending = chatMessages.value.slice();
      await loadConversation(selectedFileId.value, selectedFileGroupId.value);
      chatMessages.value = localPending;
    }
    saveMessage({
      conversation_id: currentConversationId.value,
      file_id: selectedFileId.value || undefined,
      file_group_id: selectedFileGroupId.value || undefined,
      role: "user",
      content: userContent,
    }).catch(() => {});

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
    // 对话上下文按当前作用域取：
    // - 文件组 → 聚合组内所有文件的正文（而不是编辑框里那一份）
    // - 单个文件 → 该文件正文
    // - 全图 → 退回编辑框内容
    // 取不到时静默回退，保证对话不因上下文缺失而失败。
    let contextMarkdown = markdown.value;
    if (selectedFileGroupId.value || selectedFileId.value) {
      try {
        const res = await getFilesMarkdown({
          file_id: selectedFileId.value || undefined,
          file_group_id: selectedFileGroupId.value || undefined,
          user_id: currentUserId(),
        });
        if (res.markdown.trim()) contextMarkdown = res.markdown;
      } catch {
        /* 回退到编辑框内容 */
      }
    }
    const r = await chatWithContext({
      user_message: msg || "请分析这张图片",
      conversation_id: currentConversationId.value,
      markdown: contextMarkdown,
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
      await persistEditedMarkdown(r.edited_markdown);
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
  const host = graphCanvas.value;
  const measureEl = graphRoot.value;
  if (!host || !measureEl) return;
  const rect = measureEl.getBoundingClientRect();
  const width = Math.max(Math.round(rect.width), 1);
  const height = Math.max(Math.round(rect.height), 1);
  // 清掉上一次实例可能残留的 DOM（G6 destroy 不保证移除全部画布）
  host.replaceChildren();
  graph = new Graph({
    container: host,
    width,
    height,
    autoFit: "view",
    // fitView / autoFit 计算缩放时会把 padding 作为四周留白内缩掉。
    // 默认是 0，会让适配后的内容顶满画布、节点与标签贴着边框（看起来像被裁掉）。
    padding: 64,
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
  // 注意：这里不 await render()。
  // 空的 force 布局 render 在 G6 v5 下可能不 resolve，await 会把
  // onMounted / fetchAllGraph 整条链路卡死（表现为一直「加载中…」）。
  // 真正有数据的 render 由 renderGraph 负责并 await。
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
  // 悬停高亮。
  //
  // 注意：必须只操作「当前图上真实存在的元素」。
  // graphRawData 保存的是完整数据，而画面可能被状态筛选、专注模式等过滤过，
  // 直接遍历 graphRawData 会拿被过滤掉的 id 去调 setElementState，
  // G6 内部会走 getNode(id) 并抛 "Node not found for id: xxx"。
  graph.on("node:pointerenter", (e: any) => {
    const id = resolveNodeIdFromEvent(e);
    if (!id || !graph) return;
    const live = liveGraphData();
    if (!live) return;
    const nodeIds = new Set(live.nodes.map((n: any) => n.id));
    if (!nodeIds.has(id)) return;
    applyElementState(id, "highlight");
    live.edges
      .filter((edge: any) => edge.source === id || edge.target === id)
      .forEach((edge: any) => {
        const edgeId = edge.id || `${edge.source}-${edge.target}-${edge.label}`;
        applyElementState(edgeId, "highlight");
      });
  });
  graph.on("node:pointerleave", () => {
    const live = liveGraphData();
    if (!live) return;
    live.nodes.forEach((node: any) => applyElementState(node.id, []));
    live.edges.forEach((edge: any) => {
      const edgeId = edge.id || `${edge.source}-${edge.target}-${edge.label}`;
      applyElementState(edgeId, []);
    });
  });
  graph.on("canvas:click", () => {
    selectedNodeDetail.value = null;
  });
}

/**
 * 取「当前画布上真实存在的」图数据。
 *
 * graphRawData 是完整数据集，而画布内容会被状态筛选、专注模式等裁剪。
 * 任何要按 id 操作画布元素的地方（高亮、定位、缩放）都应基于这份数据，
 * 否则会拿到已被过滤掉的 id，触发 G6 的 "Node not found for id"。
 */
function liveGraphData(): { nodes: any[]; edges: any[] } | null {
  if (!graph) return null;
  const d = graph.getData() as unknown as { nodes?: any[]; edges?: any[] };
  if (!d) return null;
  return { nodes: d.nodes ?? [], edges: d.edges ?? [] };
}

/**
 * 安全地设置元素状态。
 * 元素不存在时静默跳过——画布数据在重渲染间隔里可能已经变了，
 * 这里不该因为一个过期 id 就中断整段逻辑。
 */
function applyElementState(id: string, state: string | string[]) {
  if (!graph || !id) return;
  const live = liveGraphData();
  if (!live) return;
  const exists =
    live.nodes.some((n: any) => n.id === id) || live.edges.some((e: any) => e.id === id);
  if (!exists) return;
  try {
    graph.setElementState(id, state as any);
  } catch {
    // G6 内部仍可能因并发重渲染抛错，忽略即可，不影响主流程
  }
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

/** 上一次渲染的节点 id 集合指纹，用于判断数据集是否真的换了 */
let lastRenderedNodeKey = "";

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

  // 切换文件 / 文件组时是一份全新的数据集，必须让布局从头算一遍。
  //
  // 之前这里在「有任一节点带 x/y」时就用 preset 布局。但 G6 v5 的 setData
  // 会把上一批节点的坐标残留在画布上，新数据集的节点 id 与旧的毫无关系，
  // 于是所有新节点都落在残留坐标（多半是同一个点或原图位置）上，表现为
  // 「节点重叠在一起」。只有确实存在预计算坐标（扇形展开等）时才该用 preset，
  // 而那种情况必须保证「所有」节点都有坐标。
  const allHavePosition =
    styled.nodes.length > 0 && styled.nodes.every((n: any) => n.x !== undefined && n.y !== undefined);

  // 数据集变化时先把画布清空，再灌入新数据。
  //
  // 为什么不清空而直接 setData：G6 v5 的 setData 会保留上一批节点的坐标，
  // 新旧数据集节点 id 毫无关系，新节点就会全部堆在残留坐标上（节点重叠）。
  // 清空这一步必须在同一个实例上同步完成，不能 destroy 后重建 ——
  // 重建会让布局/交互相的异步任务拿着旧 id 继续跑并抛
  // "Unknown element type of id"，且重建过程中的 render 可能不 resolve。
  const incomingIds = styled.nodes.map((n: any) => n.id).sort().join("|");
  const datasetChanged = incomingIds !== lastRenderedNodeKey;
  lastRenderedNodeKey = incomingIds;

  // 切换数据集时，上一轮的力导向布局可能仍在跑（maxIteration 1800）。
  // 它会拿旧数据集的 id 去写坐标，此时模型里已没有这些 id，
  // G6 内部 getNode(id) 就会抛 "Node not found for id: xxx" 并刷屏。
  // 先停掉布局，再灌入新数据，最后才重新布局 —— 顺序不能颠倒。
  if (datasetChanged) {
    try {
      graph.stopLayout();
    } catch {
      // 布局尚未启动时 stop 可能抛错，忽略
    }
    // 清空模型，确保随后 setData 不会与残留元素混在一起
    try {
      graph.setData({ nodes: [], edges: [] } as any);
    } catch {
      // 忽略清空失败
    }
    // 让被中断的渲染任务先落地，避免与下面的 setData 竞争
    await new Promise((r) => requestAnimationFrame(() => r(null)));
  }

  graph.setLayout(allHavePosition ? { type: "preset", padding: 50 } : getLayoutConfig(selectedLayout));
  graph.setData(styled as any);

  // 不要 await render()。
  //
  // d3-force 在「动画模式」下用 setInterval(0) 逐帧迭代，要跑满 maxIteration
  // 次才触发 endCallback；浏览器会节流后台/高负载的定时器，这个 Promise
  // 可能几十秒都不 resolve。而调用方（selectFile / handleRefresh）是
  // `await renderGraph(...)` 后才把 isSwitching/isRefreshing 置回 false 的，
  // 于是标题会一直卡在「加载中…」。
  // 这里改为不阻塞：渲染继续进行，界面切换立刻结束。
  void graph.render();

  // 布局结束后把视图适配进画布。
  //
  // 为什么不能只靠构造时的 autoFit: "view"：
  // autoFit 只在**创建实例时**生效一次；之后每次 setData 都换了一整套节点，
  // 新坐标与视口尺寸不匹配（大图上节点跑到画布外，用户只看到一部分）。
  //
  // 为什么不在下面直接调 fitView：
  // 上面刻意没有 await render()，此刻布局仍在跑。用一个「还没收敛」的
  // 中间包围盒去算缩放，会得出错误的比例——实测出现过把图**放大**到只看得见
  // 四五个节点的情况（fitView 用的是当时的 bounds，而节点还在向外扩散）。
  // 所以改为监听 G6 的 afterlayout 事件：只有布局真正结束才适配，
  // 既不用轮询猜时机，也不会被后续坐标更新打断。
  void fitWhenLayoutSettled();
}

/**
 * 在布局结束后把整图适配进视口。
 *
 * 用 G6 的 `afterlayout` 事件而不是轮询坐标：
 * - 事件在布局真正结束时触发，此时坐标已最终确定，算出的 scale 才正确；
 * - 不需要在页面里定时轮询，避免与后续 renderGraph 调用互相干扰。
 *
 * 兜底：若事件始终没来（例如节点为空、布局被 stopLayout 提前停掉），
 * 超时后仍尝试适配一次。
 */
function fitWhenLayoutSettled(): void {
  const g = graph;
  if (!g) return;

  let done = false;
  const fit = () => {
    if (done) return;
    done = true;
    fitGraph();
  };

  try {
    (g as any).on("afterlayout", fit);
  } catch {
    fit();
    return;
  }

  // 兜底：空数据 / 布局被中断时 afterlayout 可能不触发
  setTimeout(fit, 4000);
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
    // addData 是同步的，新节点会立刻进入模型（此后按 id 操作才安全）。
    graph.addData(s as any);
    // 与 renderGraph 同样的原因：不能 await render()。
    // d3-force 动画模式要跑满 maxIteration 才 resolve，期间状态栏会一直停在
    // 「展开…」，且异常也不会进入 catch。改为不阻塞，渲染在后台继续完成。
    void graph.render();
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

// 支持的学习资料格式。图片走后端多模态识图，其余走本地解析。
const SUPPORTED_UPLOAD_EXTENSIONS = [
  ".md", ".markdown", ".txt",
  ".docx", ".pdf",
  ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp",
];
const MAX_UPLOAD_BYTES = 20 * 1024 * 1024;

function fileExtension(name: string): string {
  const i = name.lastIndexOf(".");
  return i >= 0 ? name.slice(i).toLowerCase() : "";
}

/**
 * 读取任意受支持格式的文件并转成 Markdown。
 * Markdown/文本直接本地读取（不占网络与额度），其余交给后端转换。
 */
async function readFileAsMarkdown(file: File): Promise<string> {
  const ext = fileExtension(file.name);
  if (ext === ".md" || ext === ".markdown" || ext === ".txt" || file.type === "text/markdown") {
    return await file.text();
  }
  // 传入当前用户，后端会把原件归档到该用户工作区的 original/ 目录，
  // 之后可在 AI 工作区里下载到与上传时完全一致的原始文件。
  const res = await extractToMarkdown(file, currentUserId());
  return res.markdown ?? "";
}

function validateUploadFile(file: File): string {
  const ext = fileExtension(file.name);
  if (!SUPPORTED_UPLOAD_EXTENSIONS.includes(ext)) {
    return `不支持 ${ext || "该"} 格式。支持：Markdown、txt、Word(.docx)、PDF、图片`;
  }
  if (file.size > MAX_UPLOAD_BYTES) {
    return `文件过大（>${MAX_UPLOAD_BYTES / 1024 / 1024}MB），请拆分后上传`;
  }
  return "";
}

async function handleImportMarkdownFile(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input?.files?.[0];
  if (!file) return;
  const invalid = validateUploadFile(file);
  if (invalid) {
    statusText.value = invalid;
    input.value = "";
    return;
  }
  isLoading.value = true;
  statusText.value = `正在解析：${file.name}…`;
  try {
    markdown.value = await readFileAsMarkdown(file);
    importedFileName.value = file.name;
    statusText.value = `已导入：${file.name}`;
  } catch (err) {
    statusText.value = `解析失败：${(err as Error).message}`;
  } finally {
    isLoading.value = false;
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
      const invalid = validateUploadFile(f);
      if (invalid) {
        statusText.value = `${f.name}：${invalid}`;
        continue;
      }
      try {
        // docx/pdf/图片会先在后端转成 Markdown，再进入图谱流水线
        const c = await readFileAsMarkdown(f);
        if (!c.trim()) {
          statusText.value = `${f.name}：未提取到内容，已跳过`;
          continue;
        }
        await uploadNoteLangChain({
          markdown: c,
          user_id: currentUserId(),
          file_group_id: gid,
        });
      } catch (err) {
        statusText.value = `${f.name} 解析失败：${(err as Error).message}`;
      }
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
  const target = concept.value.trim();
  isNavigating.value = true;
  statusText.value = "计算逆向学习路径…";
  try {
    const r = await getGraphPath(target, currentUserId(), maxDepth.value);
    const relatedNodes = r.all_related?.nodes ?? [];
    const depTree = r.dependency_tree ?? [];
    const meta = r.meta;

    // 深度被后端上限截断时如实告知，避免「设了 50 却按 12 算」的静默降级
    if (meta?.depth_clamped) {
      pushToast(
        `深度已按上限 ${meta.max_depth_limit} 计算`,
        "warn",
        `你设置的是 ${meta.requested_depth} 层，超出当前上限（${meta.max_depth_limit} 层）后不再向外扩展。`,
      );
    }

    // 判定「目标概念是否真的存在于图谱中」。
    // 后端在概念不存在时会返回空壳（nodes/edges 均为空），旧逻辑会因此
    // 回退去渲染并重排整张图谱 —— 那是与目标无关的无用排序。
    const hasRelated = relatedNodes.length > 0;
    const pathsWithContent = (r.paths ?? []).filter(
      (p) => (p?.nodes?.length ?? 0) > 0 || (p?.edges?.length ?? 0) > 0,
    );
    const matched = hasRelated || pathsWithContent.length > 0 || depTree.length > 0;

    if (!matched) {
      // 未命中：不改变图谱布局，只给出明确反馈（toast 自动淡出，不打断操作）
      const hint = `图谱中未找到「${target}」`;
      statusText.value = `${hint}，请先在笔记中补充该知识点再导航`;
      pushToast(
        hint,
        "warn",
        "可能原因：笔记里还没写过这个概念，或者图谱用的是别的叫法。可先用「搜索概念…」确认确切名称。",
        6000,
      );
      chatMessages.value.push({
        role: "ai",
        content: `### 未找到「${target}」

当前知识图谱里没有「${target}」这个概念，因此无法生成逆向学习路径。

可能的原因：
- 你的笔记里还没写过这个概念（未上传相关文件或未生成图谱）
- 用词不同：图谱用的是别的名称（例如「二叉排序树」而不是「二叉树」）

建议：先用「搜索概念…」定位一下确切名称，或把相关笔记上传后重新生成图谱。`,
      });
      return;
    }

    // 强依赖（PREREQUISITE_OF）是真正的学习先后关系；
    // 弱关联只是「相关」，没有先后语义，必须分开告知，避免误导。
    const strongCount = meta?.strong_count ?? depTree.filter((n) => n.strength === "strong").length;
    const weakCount = meta?.weak_count ?? depTree.filter((n) => n.strength === "weak").length;

    await renderGraph(
      hasRelated ? r.all_related! : graphRawData,
      true,
      hasRelated ? [r.all_related!] : pathsWithContent,
      "dagre",
    );
    if (depTree.length) {
      const g = await getLearningPath({
        target_concept: target,
        dependency_tree_json: JSON.stringify(depTree),
        graph_nodes_json: JSON.stringify(relatedNodes.length ? relatedNodes : graphRawData.nodes),
      }).catch(() => null);
      if (g?.guidance) {
        chatMessages.value.push({ role: "ai", content: g.guidance });
      }
    }

    statusText.value = `专注模式：${strongCount} 个前置依赖，${weakCount} 个相关概念`;

    // 结果构成提示：只有弱关联时特别说明，因为那不代表「应当先学」
    if (strongCount === 0 && weakCount > 0) {
      pushToast(
        `找到 ${weakCount} 个相关概念，但没有前置依赖`,
        "info",
        "这些概念只是与目标「相关」（RELATED_TO / SUPPLEMENTS），不代表必须先学它们。",
      );
    } else if (meta?.weak_truncated) {
      pushToast(
        `相关概念较多，已截取前 ${weakCount} 个`,
        "info",
        "可减小深度以获得更聚焦的结果。",
      );
    }
  } catch (err) {
    const msg = (err as Error).message;
    statusText.value = `路径查询失败：${msg}`;
    pushToast("路径查询失败", "error", msg, 6000);
  } finally {
    isNavigating.value = false;
  }
}

/**
 * 进入专注模式（画布工具栏按钮）。
 *
 * 专注模式只是「打开开关」，真正的内容要等你点某个节点才会显示它以中心
 * 的 2 层邻域。直接开开关画布毫无变化，容易被误认为按钮无效，因此这里
 * 给出明确引导；若已有选中的概念，则直接聚焦它，省去再点一次。
 */
async function enterFocusMode() {
  isFocusMode.value = true;
  const pending = focusedNodeId.value || concept.value.trim();
  if (pending) {
    const exists = graphRawData.nodes.some((n) => n.id === pending);
    if (exists) {
      await focusNode(pending);
      return;
    }
  }
  pushToast(
    "专注模式已开启，请点击任意节点",
    "info",
    "将只显示该概念周围 2 层邻居，再点其他节点可继续切换中心。",
    6000,
  );
  statusText.value = "专注模式：点击任意节点查看它的邻域";
}

async function resetFocus() {
  isFocusMode.value = false;
  focusedNodeId.value = "";
  // 两侧共用状态，退出时一并清空输入框，保持界面一致
  concept.value = "";
  await renderGraph(graphRawData, false, [], "force");
  isNavigating.value = false;
  statusText.value = "已退出专注模式";
}

async function focusNode(nodeId: string) {
  // 防御：nodeId 必须真实存在于当前图中。
  // G6/graphlib 在收到不存在的 id 时会抛 "Node not found for id: xxx"，
  // 若直接透传，用户看到的就是一条原始异常。这里提前拦截并友好提示。
  const exists = graphRawData.nodes.some((n) => n.id === nodeId);
  if (!exists) {
    statusText.value = `图谱中不存在节点「${nodeId}」`;
    pushToast(`图谱中不存在「${nodeId}」`, "warn", "该节点可能已被删除或尚未生成，请重新生成图谱。");
    return;
  }

  const focused = localFocusData(nodeId);
  isFocusMode.value = true;
  focusedNodeId.value = nodeId;
  // 同步侧边栏输入框：专注模式与逆向导航共用状态，
  // 不同步的话会出现「画布已聚焦 A，侧边栏还写着 B」的割裂感。
  concept.value = nodeId;
  await renderGraph(graphRawData, true, [focused], "force");

  // 渲染完成后再次确认（筛选/聚焦可能把节点排除在外）
  const rendered = new Set(focused.nodes.map((n) => n.id));
  if (!rendered.has(nodeId)) {
    statusText.value = `「${nodeId}」在当前视图范围内没有可展示的关联`;
    return;
  }

  try {
    await (graph as any)?.focusElement?.(nodeId, { animation: { duration: 500 } });
  } catch (err) {
    // 焦点动画失败不影响聚焦本身，仅记录，不打断用户
    console.warn("focusElement failed:", err);
  }
  statusText.value = `专注模式：${focused.nodes.length} 个相关知识点`;
}

async function searchGraph() {
  const keyword = graphSearch.value.trim().toLowerCase();
  if (!keyword) return;
  const node = graphRawData.nodes.find((item) => (item.label || item.id).toLowerCase().includes(keyword));
  if (!node) {
    const kw = graphSearch.value.trim();
    statusText.value = `未找到概念「${kw}」`;
    pushToast(`未找到概念「${kw}」`, "warn", "试试更短的关键词，或先上传包含该知识点的笔记。");
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

/**
 * 把整图适配进画布（「适应屏幕」按钮与布局结束后的自动适配共用）。
 *
 * 关键点：G6 的 fitView 会把 context.options.padding 当作四周留白
 * （viewport.js 里 `const [top,right,bottom,left] = this.padding`，
 * 然后按内缩后的区域算 scale）。默认 padding 是 0，
 * 于是适配完内容正好顶满画布——节点和标签贴在边缘、看起来像被裁掉。
 * 所以这里显式传入宽松的 padding。
 *
 * 另外：`direction: "both"` 会按 x/y 中较小的比例缩放，保证两个方向都装得下。
 */
function fitGraph(): void {
  if (!graph) return;
  const g: any = graph;

  // 适配要跑两遍：布局停止后元素包围盒可能还会因为「边/标签重算」而变化一次，
  // 只适配一遍会让最后变大的节点探出画面（实测有节点顶到画布上边缘之外）。
  const doFit = () => {
    if (!graph) return;
    try {
      const p = g.fitView?.({ when: "always", direction: "both" });
      // padding 已在 Graph 构造项里设为 64，这里不必重复传入
      return p && typeof p.catch === "function" ? p.catch(() => {}) : undefined;
    } catch {
      return undefined;
    }
  };

  requestAnimationFrame(() => {
    void Promise.resolve(doFit()).then(() => {
      // 再等一帧做第二次微调，吸收上一步之后发生的尺寸变化
      requestAnimationFrame(() => void doFit());
    });
  });
}

function zoomGraph(ratio: number) {
  void (graph as any)?.zoomBy?.(ratio);
}

onMounted(async () => {
  await initGraph();
  // 恢复上次登录的用户，避免刷新后文件列表按 default_user 查询而显示为空
  if (loggedInUserId.value) {
    userId.value = loggedInUserId.value;
  }
  await loadFileList({ silent: true });
  // 恢复「全图」作用域下的历史对话，并刷新状态栏。
  // 必须放在 fetchAllGraph() 之前：大图渲染（G6 力导向，数百节点）可能长时间不返回，
  // 若在它之后 await，聊天历史会被一直阻塞而显示为空。
  if (loggedInUserId.value) {
    await loadConversation("", "");
    statusText.value =
      files.value.length === 0 && fileGroups.value.length === 0
        ? `新用户「${loggedInUserId.value}」已创建，上传 MD 文件开始使用`
        : `欢迎回来「${loggedInUserId.value}」，${files.value.length} 个文件、${fileGroups.value.length} 个文件组`;
  }
  // 图谱渲染失败或卡住都不应影响上面的会话恢复与状态展示
  try {
    await fetchAllGraph();
  } catch {
    statusText.value = "图谱待生成";
  }
  if (graphRoot.value && graph) ensureResizeObserver();
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

    <!-- 自动淡出的轻提示（未命中、深度截断等） -->
    <ToastStack :toasts="toasts" @dismiss="dismissToast" />
    <!-- 左侧：折叠时收成图标栏 -->
    <SidebarRail
      v-if="leftCollapsed"
      side="left"
      :items="leftRailItems"
      @select="handleLeftRailSelect"
      @expand="expandSidebar('left')"
    />
    <!-- 左侧：文件管理（可拖拽调宽） -->
    <FileSidebar
      v-else
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
      @collapse="collapseSidebar('left')"
    />

    <!-- 折叠态下仍需能上传：图标栏里的隐藏 file input -->
    <input
      ref="leftRailUploadInput"
      class="hidden"
      type="file"
      accept=".md,.markdown,.txt,.docx,.pdf,.png,.jpg,.jpeg,.webp,.gif,.bmp"
      @change="handleLeftUploadFile"
    />

    <div
      v-if="!leftCollapsed"
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
        <!--
          graphRoot 只负责撑满父容器，绝不能让 G6 直接作为它的容器：
          G6 初始化时会改写容器的 style（把 absolute 改成 relative 并设固定宽高），
          一旦被改写，inset-0 失效，容器塌缩到内容高度，画布下方就露出白屏。
          所以真正的画布容器是内层的 graphCanvas。
        -->
        <div ref="graphRoot" class="absolute inset-0">
          <div ref="graphCanvas" class="h-full w-full" />
        </div>

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
          <button class="h-9 rounded-xl px-3 text-xs font-medium transition" :class="isFocusMode ? 'bg-indigo-600 text-white' : 'bg-slate-100 text-slate-600 hover:bg-slate-200'" @click="isFocusMode ? resetFocus() : enterFocusMode()">
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

    <!-- 右侧：折叠时收成图标栏 -->
    <SidebarRail
      v-if="rightCollapsed"
      side="right"
      :items="rightRailItems"
      @select="(k) => expandSidebar('right', k)"
      @expand="expandSidebar('right')"
    />

    <!-- 右侧：导入 / 导航 / 对话（可拖拽调宽） -->
    <aside
      v-show="!rightCollapsed"
      ref="rightPanelEl"
      :style="{ width: rightCollapsed ? '0px' : `${rightWidth}px` }"
      class="relative flex shrink-0 flex-col gap-3 overflow-hidden border-l border-gray-200 bg-[#F8FAFC] p-3"
    >
      <!-- 侧栏标题行：折叠按钮 -->
      <div class="flex shrink-0 items-center justify-between px-1">
        <span class="text-[12px] font-medium text-slate-400">学习面板</span>
        <button
          type="button"
          class="flex h-7 w-7 items-center justify-center rounded-lg text-slate-400 transition duration-200 hover:bg-gray-200 hover:text-slate-700"
          title="折叠侧栏"
          @click="collapseSidebar('right')"
        >
          <PanelRightClose class="h-4 w-4" />
        </button>
      </div>

      <div ref="importPanelEl" class="shrink-0">
        <ImportPanel
          v-model:markdown="markdown"
          v-model:user-id="userId"
          :imported-file-name="importedFileName"
          :logged-in-user-id="loggedInUserId"
          :is-loading="isLoading"
          :can-generate="canGenerate"
          @login="handleLogin"
          @logout="handleLogout"
          @import-file="handleImportMarkdownFile"
          @generate="handleUpload"
        />
      </div>

      <div ref="navPanelEl" class="shrink-0">
        <LearningNavPanel
          v-model:concept="concept"
          v-model:max-depth="maxDepth"
          :max-depth-limit="MAX_PATH_DEPTH_LIMIT"
          :can-navigate="canNavigate"
          :is-loading="isLoading"
          :is-navigating="isNavigating"
          @navigate="handlePathNavigate"
          @reset="resetFocus"
        />
      </div>

      <!--
        AI 学习导师需要占满剩余高度，但工作区面板展开时不能把它挤没。
        这里用 min-h-[300px] 给聊天区一个下限：AiChatPanel 自身样式不动，
        仍由它内部的 flex-1 负责填充。
      -->
      <div ref="chatPanelEl" class="flex min-h-[300px] flex-1 flex-col">
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
      </div>

      <WorkspacePanel
        ref="workspacePanelRef"
        :user-id="loggedInUserId"
        :library-files="files.map((f) => ({ id: f.id, name: f.name }))"
        @sync-file="handleSyncFileToWorkspace"
        @notify="statusText = $event"
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
