<script setup lang="ts">
import { Graph } from "@antv/g6";
import { computed, onBeforeUnmount, onMounted, ref, nextTick } from "vue";
import {
  explainConcept, getGraphAll, getGraphPath, getNodeNeighbors,
  uploadNoteLangChain, listUserFiles, createFileGroup,
  deleteFile, deleteFileGroup, renameFile, renameFileGroup, addFileToGroup,
  togglePinFile, togglePinFileGroup,
  chatWithContext, getLearningPath,
  getConversation, saveMessage, deleteConversation,
  type GraphResponse, type UserFile, type FileGroup,
} from "../api/graph";
import {
  buildFocusSet, buildStyledGraph, preprocessGraphData,
  getLayoutConfig, getNodeConfig, getEdgeConfig,
  type LayoutType,
} from "../graph/g6-config";
import { marked } from "marked";

function renderMarkdown(text: string): string {
  if (!text) return "";
  return marked.parse(text, { breaks: true }) as string;
}

// ========== 状态 ==========
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

const files = ref<UserFile[]>([]);
const fileGroups = ref<FileGroup[]>([]);
const selectedFileId = ref("");
const selectedFileGroupId = ref("");
const newGroupName = ref("");
const menuOpen = ref("");
const renameTarget = ref<{ type: "file" | "group"; id: string; currentName: string } | null>(null);
const renameValue = ref("");
const addFileToGroupTarget = ref("");
const showLoginPrompt = ref(false);

const selectedNodeDetail = ref<{
  id: string; label: string; type?: string; status?: string;
  reason?: string; snippets: string[]; aiExplanation: string;
} | null>(null);
const isExplaining = ref(false);

const chatMessages = ref<Array<{ role: "user" | "ai"; content: string }>>([]);
const chatInput = ref("");
const isChatting = ref(false);
const currentConversationId = ref("");

const isLightRAGMode = ref(true);
const expandedNodes = ref<Set<string>>(new Set());
const leftCollapsed = ref(false);
const rightCollapsed = ref(false);
const leftWidth = ref(240);
const rightWidth = ref(340);
const dragging = ref<"left"|"right"|null>(null);

function onDividerMousedown(side: "left"|"right", e: MouseEvent) { dragging.value = side; e.preventDefault(); document.body.style.cursor="col-resize"; document.body.style.userSelect="none"; }
window.addEventListener("mousemove", (e: MouseEvent) => {
  if (!dragging.value) return;
  const minW=160, maxW=480;
  if (dragging.value==="left") leftWidth.value=Math.min(maxW,Math.max(minW,e.clientX));
  else rightWidth.value=Math.min(maxW,Math.max(minW,window.innerWidth-e.clientX));
});
window.addEventListener("mouseup", () => {
  if (dragging.value) { dragging.value=null; document.body.style.cursor=""; document.body.style.userSelect=""; resizeGraph(); }
});

let graph: Graph | null = null;
let graphRawData: GraphResponse = { nodes: [], edges: [] };
let resizeObserver: ResizeObserver | null = null;

const canGenerate = computed(() => markdown.value.trim().length > 0);
const canNavigate = computed(() => concept.value.trim().length > 0);

function currentUserId(): string { return loggedInUserId.value || undefined as unknown as string; }
function requireLogin(): boolean { if (!loggedInUserId.value) { showLoginPrompt.value = true; return false; } return true; }

// ========== 折叠 ==========
function toggleLeft() { leftCollapsed.value = !leftCollapsed.value; resizeGraph(); }
function toggleRight() { rightCollapsed.value = !rightCollapsed.value; resizeGraph(); }
function resizeGraph() { nextTick(() => { if (graphRoot.value && graph) { const r = graphRoot.value.getBoundingClientRect(); graph.resize(r.width, r.height); } }); }

// ========== 工具函数 ==========
function tooltipHtml(reason: string) {
  return `<div style="max-width:260px;border:1px solid #E2E8F0;border-radius:12px;background:rgba(15,23,42,0.95);color:#F8FAFC;padding:10px 12px;box-shadow:0 10px 30px rgba(15,23,42,0.25);font-size:12px;line-height:1.5;">${reason || "暂无批注原因"}</div>`;
}
function extractMarkdownSnippets(concept: string, content: string): string[] {
  if (!concept.trim() || !content.trim()) return [];
  const lines = content.split(/\r?\n/); const nc = concept.toLowerCase();
  const snippets: string[] = []; const used = new Set<string>();
  for (let i = 0; i < lines.length; i++) {
    if (!lines[i].toLowerCase().includes(nc)) continue;
    const s = Math.max(0, i - 1), e = Math.min(lines.length - 1, i + 1);
    const k = `${s}-${e}`; if (used.has(k)) continue; used.add(k);
    const sn = lines.slice(s, e + 1).join("\n").trim(); if (sn) snippets.push(sn);
    if (snippets.length >= 3) break;
  }
  return snippets;
}
function buildAIExplanation(node: { label: string; status?: string; reason?: string }, snippets: string[]): string {
  const m: Record<string, string> = { correct: "该知识点逻辑基本成立。", error: "该知识点存在明显错误，需优先纠正。", supplement: "该知识点存在逻辑断层，建议补全前置知识。" };
  const st = m[node.status ?? ""] || "状态信息不足。";
  const sh = snippets.length ? `基于原笔记可提取到 ${snippets.length} 处相关描述。` : "未检索到明显原文描述。";
  const r = node.reason?.trim() ? `系统批注：${node.reason.trim()}` : "";
  return `${st}\n\n${r}\n\n${sh}\n\n建议：1.写出概念定义 2.列出2个前置知识+1个应用场景 3.用自己的话复述。`;
}
function resolveNodeIdFromEvent(event: any): string {
  const c = [event?.data?.id, event?.target?.id, event?.target?.data?.id, event?.itemId];
  const h = c.find((v: any) => typeof v === "string" && v.trim().length > 0);
  return h ? String(h) : "";
}
function scrollChatBottom() { const el = document.getElementById("chat-messages"); if (el) el.scrollTop = el.scrollHeight; }

// ========== 登录 ==========
async function handleLogin() {
  const u = userId.value.trim(); if (!u) return;
  loggedInUserId.value = u; selectedFileId.value = ""; selectedFileGroupId.value = "";
  chatMessages.value = []; currentConversationId.value = ""; showLoginPrompt.value = false;
  await loadFileList();
  statusText.value = (files.value.length === 0 && fileGroups.value.length === 0)
    ? `🆕 新用户「${u}」已创建，上传 MD 文件开始使用`
    : `✅ 欢迎回来「${u}」，${files.value.length} 个文件、${fileGroups.value.length} 个文件组`;
}

// ========== 文件管理 ==========
async function loadFileList() {
  try { const r = await listUserFiles(currentUserId() || undefined); files.value = r.files ?? []; fileGroups.value = r.file_groups ?? []; } catch (_) {}
}
async function handleCreateGroup() {
  if (!newGroupName.value.trim() || !requireLogin()) return;
  try { await createFileGroup(newGroupName.value.trim(), currentUserId() || undefined); newGroupName.value = ""; await loadFileList(); }
  catch (err) { statusText.value = `创建文件组失败：${(err as Error).message}`; }
}
async function handleDeleteFile(id: string) {
  try { await deleteFile(id, currentUserId() || undefined); if (selectedFileId.value === id) selectedFileId.value = ""; menuOpen.value = ""; await loadFileList(); }
  catch (err) { statusText.value = `删除失败：${(err as Error).message}`; }
}
async function handleDeleteGroup(id: string) {
  try { await deleteFileGroup(id, currentUserId() || undefined); if (selectedFileGroupId.value === id) selectedFileGroupId.value = ""; menuOpen.value = ""; await loadFileList(); }
  catch (err) { statusText.value = `删除失败：${(err as Error).message}`; }
}
function openRenameDialog(type: "file" | "group", id: string, name: string) { renameTarget.value = { type, id, currentName: name }; renameValue.value = name; menuOpen.value = ""; }
async function handleRename() {
  if (!renameTarget.value || !renameValue.value.trim()) return;
  try {
    if (renameTarget.value.type === "file") await renameFile(renameTarget.value.id, renameValue.value.trim(), currentUserId() || undefined);
    else await renameFileGroup(renameTarget.value.id, renameValue.value.trim(), currentUserId() || undefined);
    renameTarget.value = null; await loadFileList();
  } catch (err) { statusText.value = `改名失败：${(err as Error).message}`; }
}
async function handleTogglePin(type: "file" | "group", id: string) {
  try { if (type === "file") await togglePinFile(id, currentUserId() || undefined); else await togglePinFileGroup(id, currentUserId() || undefined); menuOpen.value = ""; await loadFileList(); }
  catch (err) { statusText.value = `操作失败：${(err as Error).message}`; }
}
async function handleAddFileToGroup(fileId: string, groupId: string) {
  try { await addFileToGroup(fileId, groupId, currentUserId() || undefined); addFileToGroupTarget.value = ""; menuOpen.value = ""; await loadFileList(); statusText.value = "文件已加入文件组"; }
  catch (err) { statusText.value = `加入文件组失败：${(err as Error).message}`; }
}

// ========== 文件选择 / 图谱加载 ==========
const isSwitching = ref(false);
async function selectFile(id: string) {
  selectedFileId.value = id; selectedFileGroupId.value = ""; isSwitching.value = true;
  const [_, __] = await Promise.all([fetchGraphByFile(id), loadConversation(id, "")]);
  isSwitching.value = false;
}
async function selectFileGroup(id: string) {
  selectedFileGroupId.value = id; selectedFileId.value = ""; isSwitching.value = true;
  const [_, __] = await Promise.all([fetchGraphByGroup(id), loadConversation("", id)]);
  isSwitching.value = false;
}
async function fetchGraphByFile(id: string) {
  try { const r = await getGraphAll({ file_id: id, user_id: currentUserId() || undefined }); graphRawData = preprocessGraphData(r.nodes, r.edges, { minConfidence: 0.6, removeSelfLoops: true, keepIsolatedNodes: false }); await renderGraph(graphRawData); }
  catch (err) { statusText.value = `加载失败：${(err as Error).message}`; }
}
async function fetchGraphByGroup(id: string) {
  try { const r = await getGraphAll({ file_group_id: id, user_id: currentUserId() || undefined }); graphRawData = preprocessGraphData(r.nodes, r.edges, { minConfidence: 0.6, removeSelfLoops: true, keepIsolatedNodes: false }); await renderGraph(graphRawData); }
  catch (err) { statusText.value = `加载失败：${(err as Error).message}`; }
}

// ========== 对话 ==========
async function loadConversation(fileId: string, fileGroupId: string) {
  try { const c = await getConversation({ file_id: fileId || undefined, file_group_id: fileGroupId || undefined, user_id: currentUserId() || undefined }); currentConversationId.value = c.id; chatMessages.value = c.messages.map(m => ({ role: m.role as "user"|"ai", content: m.content })); nextTick(() => scrollChatBottom()); }
  catch (_) { chatMessages.value = []; currentConversationId.value = ""; }
}
async function handleClearConversation() {
  if (!currentConversationId.value) return;
  try { await deleteConversation(currentConversationId.value, currentUserId() || undefined); chatMessages.value = []; currentConversationId.value = ""; }
  catch (err) { statusText.value = `清空对话失败：${(err as Error).message}`; }
}
async function sendChatMessage(imageBase64 = "") {
  const msg = chatInput.value.trim(); if (!msg && !imageBase64 || isChatting.value) return;
  chatMessages.value.push({ role: "user", content: msg || "[图片]" }); chatInput.value = ""; isChatting.value = true;
  if (!currentConversationId.value) await loadConversation(selectedFileId.value, selectedFileGroupId.value);
  saveMessage({ conversation_id: currentConversationId.value, file_id: selectedFileId.value || undefined, file_group_id: selectedFileGroupId.value || undefined, role: "user", content: msg || "[图片]" }).catch(() => {});
  try {
    const gn = JSON.stringify(graphRawData.nodes.map(n => ({ name: n.label||n.id, status: n.status, reason: n.reason, definition: n.data?.definition??"" })));
    const ge = JSON.stringify(graphRawData.edges.map(e => ({ source: e.source, target: e.target, relation: e.label, status: e.status, reason: e.reason })));
    const r = await chatWithContext({ user_message: msg || "请分析这张图片", conversation_id: currentConversationId.value, markdown: markdown.value, graph_nodes: gn, graph_edges: ge, image_base64: imageBase64 });
    chatMessages.value.push({ role: "ai", content: r.reply });
    saveMessage({ conversation_id: currentConversationId.value, file_id: selectedFileId.value || undefined, file_group_id: selectedFileGroupId.value || undefined, role: "ai", content: r.reply }).catch(() => {});
    // 如果 AI 修改了文档，同步更新 markdown
    if (r.edited_markdown) { markdown.value = r.edited_markdown; statusText.value = "AI 已修改文档，可点击生成图谱查看更新"; }
  } catch (err) { chatMessages.value.push({ role: "ai", content: `对话出错：${(err as Error).message}` }); }
  finally { isChatting.value = false; nextTick(() => scrollChatBottom()); }
}
function handleChatImageUpload(event: Event) {
  const input = event.target as HTMLInputElement; const file = input?.files?.[0]; if (!file) return;
  const reader = new FileReader();
  reader.onload = () => { const b64 = (reader.result as string).split(",")[1]; sendChatMessage(b64); };
  reader.readAsDataURL(file); input.value = "";
}

// ========== 节点详情 ==========
async function showNodeDetail(nodeId: string) {
  const node = graphRawData.nodes.find(n => n.id === nodeId); if (!node) return;
  const snippets = extractMarkdownSnippets(node.label || node.id, markdown.value);
  selectedNodeDetail.value = { id: node.id, label: node.label || node.id, type: node.type, status: node.status, reason: node.reason, snippets, aiExplanation: buildAIExplanation({ label: node.label||node.id, status: node.status, reason: node.reason }, snippets) };
  if (!markdown.value.trim()) return;
  isExplaining.value = true;
  try { const r = await explainConcept({ concept: node.label||node.id, markdown: markdown.value, user_id: currentUserId()||undefined }); if (selectedNodeDetail.value?.id === node.id) selectedNodeDetail.value.aiExplanation = r.explanation; }
  catch (err) { if (selectedNodeDetail.value?.id === node.id) selectedNodeDetail.value.aiExplanation = `讲解失败：${(err as Error).message}`; }
  finally { isExplaining.value = false; }
}

// ========== 图谱渲染 ==========
async function initGraph() {
  if (!graphRoot.value) return;
  const { width, height } = graphRoot.value.getBoundingClientRect();
  graph = new Graph({
    container: graphRoot.value, width, height, autoFit: "view", data: { nodes:[], edges:[] },
    node: getNodeConfig(), edge: getEdgeConfig(), layout: getLayoutConfig("force"),
    behaviors: ["drag-canvas", "zoom-canvas", "drag-element"],
    plugins: [{ type: "tooltip", trigger: "hover",
      enable: (e: any) => { if (e.targetType!=="node") return false; const s = String(e.target?.data?.status??""); return s==="error"||s==="supplement"; },
      getContent: (e: any) => tooltipHtml(String(e.target?.data?.reason??"")) }],
  });
  await graph.render();
  graph.on("node:click", (e: any) => { const id = resolveNodeIdFromEvent(e); if (!id) return; if (isLightRAGMode.value && !expandedNodes.value.has(id)) { void expandNodeNeighbors(id); } else { void showNodeDetail(id); } });
  graph.on("canvas:click", () => { selectedNodeDetail.value = null; });
}
async function renderGraph(data: GraphResponse, focusMode = false, pathData: GraphResponse[] = [], layoutType: LayoutType = "force") {
  if (!graph) return;
  const focusSet = focusMode ? buildFocusSet(pathData) : undefined;
  const styled = buildStyledGraph(data, focusSet);
  graph.setLayout((styled.nodes.some((n: any) => n.x !== undefined)) ? { type: "preset", padding: 50 } : getLayoutConfig(layoutType));
  graph.setData(styled as any); await graph.render();
}
async function expandNodeNeighbors(nodeId: string) {
  if (!graph) return; statusText.value = `展开 "${nodeId}" 的邻居...`;
  try {
    const nb = await getNodeNeighbors(nodeId, currentUserId()||undefined, 1);
    const existN = new Set(graphRawData.nodes.map(n=>n.id)), existE = new Set(graphRawData.edges.map(e=>e.id));
    const nn = nb.nodes.filter(n=>!existN.has(n.id)), ne = nb.edges.filter(e=>!existE.has(e.id));
    if (nn.length===0 && ne.length===0) { statusText.value=`"${nodeId}" 无更多邻居`; expandedNodes.value.add(nodeId); return; }
    graphRawData.nodes.push(...nn); graphRawData.edges.push(...ne); expandedNodes.value.add(nodeId);
    const s = buildStyledGraph({nodes:nn,edges:ne}); graph.addData(s as any); await graph.render();
    statusText.value=`已展开 ${nn.length} 节点 ${ne.length} 边`;
  } catch(e) { statusText.value=`展开失败：${(e as Error).message}`; }
}
function extractCoreNodes(data: GraphResponse, n: number): GraphResponse {
  const deg = new Map<string,number>(); data.nodes.forEach(x=>deg.set(x.id,0));
  data.edges.forEach(e=>{ deg.set(e.source,(deg.get(e.source)||0)+1); deg.set(e.target,(deg.get(e.target)||0)+1); });
  const sn = [...data.nodes].sort((a,b)=>(deg.get(b.id)||0)-(deg.get(a.id)||0)).slice(0,n);
  const ids = new Set(sn.map(x=>x.id)); return {nodes:sn, edges:data.edges.filter(e=>ids.has(e.source)&&ids.has(e.target))};
}
async function fetchAllGraph() {
  graphRawData = { nodes:[], edges:[] };
  if (selectedFileId.value) { await fetchGraphByFile(selectedFileId.value); return; }
  if (selectedFileGroupId.value) { await fetchGraphByGroup(selectedFileGroupId.value); return; }
  const raw = await getGraphAll({ user_id: currentUserId()||undefined });
  graphRawData = preprocessGraphData(raw.nodes, raw.edges, { minConfidence:0.6, removeSelfLoops:true, keepIsolatedNodes:false });
  if (isLightRAGMode.value) { graphRawData = extractCoreNodes(graphRawData, 5); expandedNodes.value.clear(); }
  await renderGraph(graphRawData);
  statusText.value = `图谱：${graphRawData.nodes.length} 节点 / ${graphRawData.edges.length} 连线${isLightRAGMode.value?" (渐进式展开)":""}`;
}

// ========== 上传 MD ==========
async function handleUpload() {
  if (!canGenerate.value) return; isLoading.value = true;
  statusText.value = "AI 诊断中（NER→校验→补全）...";
  try {
    const r = await uploadNoteLangChain({ markdown: markdown.value, user_id: currentUserId()||undefined, file_id: selectedFileId.value||undefined, file_group_id: selectedFileGroupId.value||undefined });
    if (r.file_id && !selectedFileId.value) selectedFileId.value = r.file_id as string;
    await loadFileList(); await fetchAllGraph();
    // 立即向 AI 对话面板推送诊断总结
    pushDiagnosisSummary(r);
    isNavigating.value = false; selectedNodeDetail.value = null;
  } catch(err) { statusText.value = `生成失败：${(err as Error).message}`; }
  finally { isLoading.value = false; }
}
function pushDiagnosisSummary(result: Record<string, any>) {
  const diag = result.diagnose_result as Record<string, any> | undefined;
  const nodes: Array<Record<string, any>> = diag?.nodes ?? [];
  const errors = nodes.filter(n => n.status === "error");
  const supplements = nodes.filter(n => n.status === "supplement");
  const corrects = nodes.filter(n => n.status === "correct");

  let summary = `## 📋 笔记诊断报告\n\n`;
  summary += `共识别 **${nodes.length}** 个知识点，生成 **${result.relations_count ?? 0}** 条关系。\n\n`;

  if (errors.length > 0) {
    summary += `### ❌ 发现 ${errors.length} 处错误\n`;
    errors.forEach(n => { summary += `- **${n.name}**：${n.reason || "笔记描述有误"}\n`; });
    summary += "\n";
  }
  if (supplements.length > 0) {
    summary += `### 📝 AI 补全 ${supplements.length} 处知识缺口\n`;
    supplements.forEach(n => { summary += `- **${n.name}**：${n.reason || "该前置知识在笔记中缺失"}\n`; });
    summary += "\n";
  }
  if (corrects.length > 0) {
    summary += `### ✅ 正确的知识点（${corrects.length} 个）\n`;
    summary += corrects.slice(0, 5).map(n => `- ${n.name}`).join("\n");
    if (corrects.length > 5) summary += `\n... 等 ${corrects.length - 5} 个`;
    summary += "\n\n";
  }
  if (errors.length === 0 && supplements.length === 0) {
    summary += `🎉 笔记质量很好，未发现错误或知识缺口！\n\n`;
  }
  summary += `💡 你可以在右侧对话框中让我帮你修正这些错误，或补充缺失的知识点。`;

  chatMessages.value.push({ role: "ai", content: summary });
  nextTick(() => scrollChatBottom());
}
async function handleImportMarkdownFile(event: Event) {
  const input = (event.target as HTMLInputElement); const file = input?.files?.[0]; if (!file) return;
  if (!file.name.endsWith(".md") && file.type !== "text/markdown") { statusText.value="仅支持 .md 文件"; input.value=""; return; }
  if (file.size > 2*1024*1024) { statusText.value="文件过大(>2MB)"; input.value=""; return; }
  try { markdown.value = await file.text(); importedFileName.value = file.name; statusText.value=`已导入：${file.name}`; input.value=""; }
  catch(err) { statusText.value=`读取失败：${(err as Error).message}`; input.value=""; }
}
async function handleLeftUploadFile(event: Event) {
  await handleImportMarkdownFile(event);
  if (markdown.value) await handleUpload();
}
async function handleLeftUploadFileGroup(event: Event) {
  const input = (event.target as HTMLInputElement); const fls = input?.files; if (!fls||fls.length===0) return;
  if (!requireLogin()) { input.value=""; return; }
  isLoading.value = true; statusText.value=`创建文件组并处理 ${fls.length} 个文件...`;
  try {
    const gn = `文件组_${new Date().toLocaleDateString()}`;
    const gr = await createFileGroup(gn, currentUserId()||undefined); const gid = (gr as any).group_id as string;
    for (const f of Array.from(fls)) { statusText.value=`处理：${f.name}...`; const c = await f.text(); await uploadNoteLangChain({ markdown:c, user_id:currentUserId()||undefined, file_group_id:gid }); }
    await loadFileList(); selectedFileGroupId.value = gid; await fetchGraphByGroup(gid);
    statusText.value = `文件组已创建：${fls.length} 个文件`;
  } catch(err) { statusText.value=`创建文件组失败：${(err as Error).message}`; }
  finally { isLoading.value = false; input.value = ""; }
}
async function handlePathNavigate() {
  if (!canNavigate.value) return; isNavigating.value = true; statusText.value="计算逆向学习路径...";
  try {
    const r = await getGraphPath(concept.value.trim(), currentUserId()||undefined, maxDepth.value);
    const hasRelated = r.all_related && r.all_related.nodes && r.all_related.nodes.length > 0;
    await renderGraph(hasRelated ? r.all_related! : graphRawData, true, hasRelated ? [r.all_related!] : r.paths, "force");
    if (r.dependency_tree?.length) {
      const g = await getLearningPath({ target_concept:concept.value.trim(), dependency_tree_json:JSON.stringify(r.dependency_tree), graph_nodes_json:JSON.stringify(r.all_related?.nodes??graphRawData.nodes) }).catch(()=>null);
      if (g?.guidance) { chatMessages.value.push({role:"ai",content:g.guidance}); nextTick(()=>scrollChatBottom()); }
    }
    statusText.value=`专注模式：${r.paths.length} 条路径，${r.dependency_tree?.length??0} 个前置节点`;
  } catch(err) { statusText.value=`路径查询失败：${(err as Error).message}`; }
}
async function resetFocus() { await renderGraph(graphRawData, false, [], "force"); isNavigating.value = false; statusText.value="已退出专注模式"; }

// ========== 生命周期 ==========
onMounted(async () => {
  await initGraph(); await loadFileList(); await fetchAllGraph();
  if (graphRoot.value && graph) { resizeObserver = new ResizeObserver(e => { const r = e[0]; if (r&&graph) graph.resize(r.contentRect.width, r.contentRect.height); }); resizeObserver.observe(graphRoot.value); }
});
onBeforeUnmount(() => { resizeObserver?.disconnect(); graph?.destroy(); graph = null; });
</script>

<template>
  <main class="flex h-screen w-full overflow-hidden bg-slate-100">
    <!-- ====== 左侧：文件管理 ====== -->
    <aside :style="{ width: leftCollapsed ? '0px' : leftWidth+'px' }" class="flex shrink-0 flex-col border-r border-slate-200 bg-white overflow-hidden"
      :class="{ 'border-r-0': leftCollapsed, 'transition-all duration-200': !dragging }">
      <div class="flex items-center justify-between border-b border-slate-200 px-3 py-2.5">
        <h2 class="text-sm font-semibold text-slate-800">📁 文件管理</h2>
        <button class="text-slate-400 hover:text-slate-600 text-sm" @click="toggleLeft" title="收起">◀</button>
      </div>
      <div class="px-2 py-2 space-y-1.5 border-b border-slate-100">
        <div class="flex gap-1.5">
          <label v-if="loggedInUserId" class="flex-1 cursor-pointer rounded-md bg-blue-600 px-2 py-1.5 text-center text-sm font-medium text-white hover:bg-blue-500 transition">
            📄 上传文件
            <input class="hidden" type="file" accept=".md,text/markdown" @change="handleLeftUploadFile" />
          </label>
          <label v-if="loggedInUserId" class="flex-1 cursor-pointer rounded-md bg-violet-600 px-2 py-1.5 text-center text-sm font-medium text-white hover:bg-violet-500 transition">
            📦 上传文件组
            <input class="hidden" type="file" accept=".md,text/markdown" multiple @change="handleLeftUploadFileGroup" />
          </label>
          <button v-if="!loggedInUserId" class="flex-1 rounded-md bg-slate-400 py-1.5 text-center text-sm font-medium text-white" @click="showLoginPrompt=true">📄 上传文件</button>
          <button v-if="!loggedInUserId" class="flex-1 rounded-md bg-slate-400 py-1.5 text-center text-sm font-medium text-white" @click="showLoginPrompt=true">📦 上传文件组</button>
        </div>
      </div>
      <!-- 文件组 -->
      <div class="border-b border-slate-100 px-2 py-1.5">
        <div class="flex items-center gap-1 mb-1">
          <input v-model="newGroupName" placeholder="新建文件组..." class="flex-1 rounded-md border border-slate-200 px-2 py-1 text-sm outline-none focus:border-violet-400" @keyup.enter="handleCreateGroup" />
          <button class="shrink-0 rounded-md bg-violet-100 px-2 py-1 text-sm text-violet-700 hover:bg-violet-200" @click="handleCreateGroup">+</button>
        </div>
        <div class="max-h-[18vh] space-y-0.5 overflow-y-auto">
          <div v-for="g in fileGroups" :key="g.id" class="group relative">
            <div :class="['flex items-center justify-between rounded-md px-2 py-1 text-[15px] cursor-pointer', selectedFileGroupId===g.id?'bg-violet-100 text-violet-800 font-medium':'hover:bg-slate-50 text-slate-700']"
              @click="selectFileGroup(g.id)">
              <span class="truncate flex-1">📦 {{ g.name }}</span>
              <span class="text-sm text-slate-400 mr-1">({{ g.file_ids?.length??0 }})</span>
              <button class="text-slate-300 hover:text-slate-600 text-sm" @click.stop="menuOpen=menuOpen===g.id?'':g.id">⋯</button>
            </div>
            <div v-if="menuOpen===g.id" class="absolute right-2 top-6 z-30 rounded-lg border border-slate-200 bg-white shadow-lg py-0.5 min-w-[100px]" @click.stop>
              <button class="w-full text-left px-3 py-1.5 text-[15px] text-slate-600 hover:bg-slate-50" @click="handleTogglePin('group',g.id)">📌 {{ g.pinned?'取消置顶':'置顶' }}</button>
              <button class="w-full text-left px-3 py-1.5 text-[15px] text-slate-600 hover:bg-slate-50" @click="openRenameDialog('group',g.id,g.name)">✏️ 改名</button>
              <button class="w-full text-left px-3 py-1.5 text-[15px] text-red-500 hover:bg-red-50" @click="handleDeleteGroup(g.id)">🗑️ 删除</button>
            </div>
          </div>
        </div>
      </div>
      <!-- 独立文件 -->
      <div class="flex-1 overflow-y-auto px-2 py-1.5">
        <p class="text-sm text-slate-400 px-1 mb-1">独立文件</p>
        <div class="space-y-0.5">
          <div v-for="f in files.filter(x=>!x.file_group_id)" :key="f.id" class="group relative">
            <div :class="['flex items-center justify-between rounded-md px-2 py-1 text-[15px] cursor-pointer', selectedFileId===f.id?'bg-blue-100 text-blue-800 font-medium':'hover:bg-slate-50 text-slate-700']"
              @click="selectFile(f.id)">
              <span class="truncate flex-1">{{ f.pinned?'📌':'' }}📄 {{ f.name }}</span>
              <button class="text-slate-300 hover:text-slate-600 text-sm" @click.stop="menuOpen=menuOpen===f.id?'':f.id">⋯</button>
            </div>
            <div v-if="menuOpen===f.id" class="absolute right-2 top-6 z-30 rounded-lg border border-slate-200 bg-white shadow-lg py-0.5 min-w-[100px]" @click.stop>
              <button class="w-full text-left px-3 py-1.5 text-[15px] text-slate-600 hover:bg-slate-50" @click="handleTogglePin('file',f.id)">📌 {{ f.pinned?'取消置顶':'置顶' }}</button>
              <button class="w-full text-left px-3 py-1.5 text-[15px] text-slate-600 hover:bg-slate-50" @click="openRenameDialog('file',f.id,f.name)">✏️ 改名</button>
              <button class="w-full text-left px-3 py-1.5 text-[15px] text-slate-600 hover:bg-slate-50" @click="addFileToGroupTarget=f.id;menuOpen=''">📁 加入文件组</button>
              <button class="w-full text-left px-3 py-1.5 text-[15px] text-red-500 hover:bg-red-50" @click="handleDeleteFile(f.id)">🗑️ 删除</button>
            </div>
          </div>
        </div>
        <!-- 组内文件 -->
        <template v-if="selectedFileGroupId">
          <p class="text-sm text-violet-500 px-1 mt-2 mb-1">组内文件</p>
          <div v-for="f in files.filter(x=>x.file_group_id===selectedFileGroupId)" :key="f.id"
            :class="['rounded-md px-2 py-1 text-[15px] cursor-pointer hover:bg-violet-50 text-violet-700', selectedFileId===f.id?'bg-violet-100 font-medium':'']"
            @click="selectFile(f.id)">
            📄 {{ f.name }}
          </div>
        </template>
      </div>
      <div class="border-t border-slate-200 px-2 py-1.5">
        <button class="w-full rounded-md bg-slate-100 py-1 text-sm text-slate-600 hover:bg-slate-200" @click="loadFileList">🔄 刷新</button>
      </div>
    </aside>
    <button v-if="leftCollapsed" class="w-6 shrink-0 flex items-center justify-center bg-white border-r border-slate-200 hover:bg-slate-50 transition" @click="toggleLeft" title="展开">▶</button>
    <div v-if="!leftCollapsed" class="w-1.5 shrink-0 cursor-col-resize bg-transparent hover:bg-blue-300 active:bg-blue-400 transition-colors relative group" @mousedown="onDividerMousedown('left', $event)">
      <div class="absolute inset-y-0 -left-1 -right-1" />
      <div class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-1 h-8 rounded-full bg-slate-300 group-hover:bg-blue-400 transition-colors" />
    </div>

    <!-- ====== 中间：图谱 ====== -->
    <section class="flex flex-1 flex-col overflow-hidden">
      <div class="flex items-center gap-3 border-b border-slate-200 bg-white px-3 py-2">
        <h1 class="text-sm font-semibold text-slate-800 shrink-0">Knowledge Studio</h1>
        <span :class="isSwitching?'text-[15px] text-blue-500':'text-[15px] text-slate-400'">{{ isSwitching?'加载中...':statusText }}</span>
        <div class="flex-1" />
        <button :class="['rounded-md px-2 py-1 text-sm font-medium transition', isLightRAGMode?'bg-emerald-100 text-emerald-700':'bg-slate-100 text-slate-600']" @click="isLightRAGMode=!isLightRAGMode;fetchAllGraph()">
          {{ isLightRAGMode?'渐进式展开':'显示全图' }}
        </button>
      </div>
      <div ref="graphRoot" class="flex-1 m-2 rounded-xl border border-slate-200 bg-white shadow-sm" />
    </section>

    <div v-if="!rightCollapsed" class="w-1.5 shrink-0 cursor-col-resize bg-transparent hover:bg-violet-300 active:bg-violet-400 transition-colors relative group" @mousedown="onDividerMousedown('right', $event)">
      <div class="absolute inset-y-0 -left-1 -right-1" />
      <div class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-1 h-8 rounded-full bg-slate-300 group-hover:bg-violet-400 transition-colors" />
    </div>

    <!-- ====== 右侧：笔记 + 导航 + AI对话 ====== -->
    <aside :style="{ width: rightCollapsed ? '0px' : rightWidth+'px' }" class="flex shrink-0 flex-col border-l border-slate-200 bg-white overflow-hidden"
      :class="{ 'border-l-0': rightCollapsed, 'transition-all duration-200': !dragging }">
      <!-- 上：笔记输入 + 导航 -->
      <!-- 上：笔记输入 + 导航 -->
      <div class="border-b border-slate-200 p-3 space-y-2 overflow-y-auto shrink-0" style="max-height:42%">
        <div>
          <div class="flex items-center justify-between mb-1">
            <div class="flex items-center gap-1">
              <button class="w-5 h-5 rounded-full bg-slate-100 flex items-center justify-center text-slate-400 hover:bg-slate-200 text-sm" @click="toggleRight" title="收起">▶</button>
              <span class="text-sm font-semibold text-slate-700">📝 笔记输入</span>
            </div>
            <label class="cursor-pointer rounded-md border border-slate-200 bg-slate-50 px-2 py-0.5 text-sm text-slate-500 hover:bg-white">导入.md<input class="hidden" type="file" accept=".md,text/markdown" @change="handleImportMarkdownFile" /></label>
          </div>
          <span v-if="importedFileName" class="text-sm text-slate-400">📄 {{ importedFileName }}</span>
          <textarea v-model="markdown" class="h-20 w-full resize-none rounded-lg border border-slate-200 bg-slate-50 p-2 text-[15px] text-slate-700 outline-none focus:border-blue-400" placeholder="粘贴 Markdown 笔记..." />
          <div class="mt-1.5 flex items-center gap-1.5">
            <input v-model="userId" class="w-24 rounded-md border border-slate-200 px-2 py-1 text-sm outline-none focus:border-blue-400" placeholder="user_id" />
            <button class="rounded-md bg-emerald-600 px-2 py-1 text-sm font-medium text-white hover:bg-emerald-500" @click="handleLogin">登录</button>
            <button :disabled="!canGenerate||isLoading" class="flex-1 rounded-md bg-slate-900 py-1 text-sm font-medium text-white hover:bg-slate-800 disabled:opacity-50" @click="handleUpload">{{ isLoading?'诊断中...':'生成图谱' }}</button>
          </div>
          <p v-if="loggedInUserId" class="mt-1 text-sm text-emerald-600">✅ 当前：{{ loggedInUserId }}</p>
        </div>
        <div>
          <span class="text-sm font-semibold text-slate-700">🔍 逆向导航</span>
          <div class="flex gap-1 mt-1">
            <input v-model="concept" class="flex-1 rounded-md border border-slate-200 px-2 py-1 text-sm outline-none focus:border-violet-400" placeholder="目标概念" />
            <input v-model.number="maxDepth" type="number" min="1" max="6" class="w-12 rounded-md border border-slate-200 px-1 py-1 text-sm text-center outline-none" placeholder="3" />
          </div>
          <div class="flex gap-1.5 mt-1">
            <button :disabled="!canNavigate||isLoading" class="flex-1 rounded-md bg-violet-600 py-1 text-sm font-medium text-white hover:bg-violet-500 disabled:opacity-50" @click="handlePathNavigate">逆向导航</button>
            <button v-if="isNavigating" class="rounded-md border border-slate-200 bg-white px-2 py-1 text-sm text-slate-600 hover:bg-slate-50" @click="resetFocus">退出</button>
          </div>
        </div>
        <!-- 节点详情 -->
        <div v-if="selectedNodeDetail" class="rounded-lg border border-violet-200 bg-violet-50/60 p-2 text-sm">
          <div class="flex items-center justify-between mb-0.5">
            <span class="font-semibold text-violet-800 truncate">{{ selectedNodeDetail.label }}</span>
            <button class="text-violet-400 hover:text-violet-600" @click="selectedNodeDetail=null">×</button>
          </div>
          <span :class="['inline-block rounded px-1 py-0.5 text-[13px] font-medium mb-1', selectedNodeDetail.status==='error'?'bg-red-100 text-red-700':selectedNodeDetail.status==='supplement'?'bg-violet-200 text-violet-800':'bg-slate-100 text-slate-600']">{{ selectedNodeDetail.status||'unknown' }}</span>
          <div v-if="selectedNodeDetail.reason" class="text-violet-700 mb-1">💡 {{ selectedNodeDetail.reason }}</div>
          <p v-if="isExplaining" class="text-violet-400">讲解生成中...</p>
          <p v-else class="text-slate-600 whitespace-pre-wrap leading-relaxed">{{ selectedNodeDetail.aiExplanation }}</p>
        </div>
      </div>
      <!-- 下：AI 对话 -->
      <div class="flex flex-1 flex-col overflow-hidden border-t border-slate-200">
        <div class="flex items-center justify-between px-3 py-1.5 border-b border-slate-100">
          <span class="text-[15px] font-semibold text-slate-800">🤖 AI 导师</span>
          <button v-if="currentConversationId" class="text-sm text-slate-400 hover:text-red-500" @click="handleClearConversation">清空</button>
        </div>
        <div id="chat-messages" class="flex-1 space-y-1 overflow-y-auto p-2">
          <div v-if="chatMessages.length===0" class="py-6 text-center text-sm text-slate-400">上传笔记后，AI 导师可基于图谱内容回答。</div>
          <div v-for="(m,i) in chatMessages" :key="i">
            <div v-if="m.role==='user'" class="rounded-lg px-3 py-1.5 text-[15px] max-w-[90%] ml-auto bg-blue-600 text-white">{{ m.content }}</div>
            <div v-else class="rounded-lg px-3 py-2 text-[15px] max-w-[95%] bg-slate-100 text-slate-700 markdown-body" v-html="renderMarkdown(m.content)"></div>
          </div>
          <div v-if="isChatting" class="text-sm text-slate-400">AI 思考中...</div>
        </div>
        <div class="border-t border-slate-200 p-2 flex gap-1.5 items-center">
          <label class="cursor-pointer text-slate-400 hover:text-slate-600 text-lg leading-none" title="上传图片">
            🖼️
            <input class="hidden" type="file" accept="image/*" @change="handleChatImageUpload" />
          </label>
          <input v-model="chatInput" @keyup.enter="sendChatMessage()" class="flex-1 rounded-md border border-slate-200 px-2 py-1 text-[15px] outline-none focus:border-blue-400" placeholder="输入消息..." :disabled="isChatting" />
          <button @click="sendChatMessage()" :disabled="(!chatInput.trim()&&!isChatting)||isChatting" class="rounded-md bg-blue-600 px-3 py-1 text-[15px] text-white hover:bg-blue-500 disabled:opacity-50">发送</button>
        </div>
      </div>
    </aside>
    <button v-if="rightCollapsed" class="w-6 shrink-0 flex items-center justify-center bg-white border-l border-slate-200 hover:bg-slate-50 transition" @click="toggleRight" title="展开">◀</button>

    <!-- 弹窗：未登录提示 -->
    <div v-if="showLoginPrompt" class="fixed inset-0 z-50 flex items-center justify-center bg-black/20" @click.self="showLoginPrompt=false">
      <div class="rounded-xl border border-slate-200 bg-white p-5 shadow-xl w-[300px] text-center">
        <p class="text-2xl mb-2">🔐</p>
        <p class="text-sm font-semibold text-slate-800 mb-1">需要先登录</p>
        <p class="text-sm text-slate-500 mb-4">长时文件存储需要输入 user_id 并点击登录。<br/>💡 右侧笔记区可直接使用，无需登录。</p>
        <button class="w-full rounded-lg bg-blue-600 py-2 text-sm text-white hover:bg-blue-500" @click="showLoginPrompt=false">知道了</button>
      </div>
    </div>
    <!-- 弹窗：加入文件组 -->
    <div v-if="addFileToGroupTarget" class="fixed inset-0 z-50 flex items-center justify-center bg-black/20" @click.self="addFileToGroupTarget=''">
      <div class="rounded-xl border border-slate-200 bg-white p-4 shadow-xl w-[260px]">
        <p class="text-sm font-semibold text-slate-800 mb-2">选择目标文件组</p>
        <div class="max-h-[180px] space-y-1 overflow-y-auto mb-3">
          <button v-for="g in fileGroups" :key="g.id" class="w-full text-left rounded-md px-2 py-1.5 text-[15px] text-slate-700 hover:bg-violet-50" @click="handleAddFileToGroup(addFileToGroupTarget,g.id)">📦 {{ g.name }}</button>
          <p v-if="fileGroups.length===0" class="text-[15px] text-slate-400 px-2">暂无文件组，请先创建</p>
        </div>
        <button class="w-full rounded-md bg-slate-100 py-1.5 text-[15px] text-slate-600 hover:bg-slate-200" @click="addFileToGroupTarget=''">取消</button>
      </div>
    </div>
    <!-- 弹窗：改名 -->
    <div v-if="renameTarget" class="fixed inset-0 z-50 flex items-center justify-center bg-black/20" @click.self="renameTarget=null">
      <div class="rounded-xl border border-slate-200 bg-white p-4 shadow-xl w-[250px]">
        <p class="text-sm font-semibold text-slate-800 mb-2">{{ renameTarget.type==='file'?'重命名文件':'重命名文件组' }}</p>
        <input v-model="renameValue" class="w-full rounded-lg border border-slate-200 px-2.5 py-1.5 text-sm outline-none focus:border-blue-400 mb-3" @keyup.enter="handleRename" />
        <div class="flex gap-2">
          <button class="flex-1 rounded-md bg-slate-100 py-1.5 text-[15px] text-slate-600 hover:bg-slate-200" @click="renameTarget=null">取消</button>
          <button class="flex-1 rounded-md bg-blue-600 py-1.5 text-[15px] text-white hover:bg-blue-500" @click="handleRename">确认</button>
        </div>
      </div>
    </div>
    <!-- 加载遮罩 -->
    <div v-if="isLoading" class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/30 backdrop-blur-[2px]">
      <div class="rounded-2xl border border-white/20 bg-slate-900/90 px-8 py-6 text-center text-white shadow-2xl">
        <div class="mx-auto h-10 w-10 animate-spin rounded-full border-2 border-white/20 border-t-white" />
        <p class="mt-3 text-sm">AI 正在生成知识图谱...</p>
      </div>
    </div>
  </main>
</template>
