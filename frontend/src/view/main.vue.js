import { Graph } from "@antv/g6";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { marked } from "marked";
import { addFileToGroup, chatWithContext, createFileGroup, deleteConversation, deleteFile, deleteFileGroup, explainConcept, getConversation, getGraphAll, getGraphPath, getLearningPath, getNodeNeighbors, listUserFiles, renameFile, renameFileGroup, saveMessage, togglePinFile, togglePinFileGroup, uploadNoteLangChain, } from "../api/graph";
import { buildFocusSet, buildStyledGraph, getEdgeConfig, getLayoutConfig, getNodeConfig, preprocessGraphData, } from "../graph/g6-config";
import FileSidebar from "../components/FileSidebar.vue";
import ImportPanel from "../components/ImportPanel.vue";
import LearningNavPanel from "../components/LearningNavPanel.vue";
import AiChatPanel from "../components/AiChatPanel.vue";
function renderMarkdown(text) {
    if (!text)
        return "";
    return marked.parse(text, { breaks: true });
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
const graphRoot = ref(null);
const selectedFileId = ref("");
const selectedFileGroupId = ref("");
const files = ref([]);
const fileGroups = ref([]);
const newGroupName = ref("");
const menuOpen = ref("");
const renameTarget = ref(null);
const renameValue = ref("");
const addFileToGroupTarget = ref("");
const showLoginPrompt = ref(false);
const isSwitching = ref(false);
const isRefreshing = ref(false);
const selectedNodeDetail = ref(null);
const isExplaining = ref(false);
const chatMessages = ref([]);
const chatInput = ref("");
const isChatting = ref(false);
const currentConversationId = ref("");
const isLightRAGMode = ref(true);
const expandedNodes = ref(new Set());
const graphSearch = ref("");
const graphStatusFilter = ref("all");
const graphLayoutMode = ref("auto");
const activeLayout = ref("force");
const isFocusMode = ref(false);
const focusedNodeId = ref("");
const leftWidth = ref(280);
const rightWidth = ref(340);
const dragging = ref(null);
function onDividerMousedown(side, e) {
    dragging.value = side;
    e.preventDefault();
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
}
function onDividerMousemove(e) {
    if (!dragging.value)
        return;
    const minW = 200;
    const maxW = 480;
    if (dragging.value === "left") {
        leftWidth.value = Math.min(maxW, Math.max(minW, e.clientX));
    }
    else {
        rightWidth.value = Math.min(maxW, Math.max(minW, window.innerWidth - e.clientX));
    }
}
function onDividerMouseup() {
    if (!dragging.value)
        return;
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
let graph = null;
let graphRawData = { nodes: [], edges: [] };
let resizeObserver = null;
const canGenerate = computed(() => markdown.value.trim().length > 0);
const canNavigate = computed(() => concept.value.trim().length > 0);
function currentUserId() {
    return loggedInUserId.value || undefined;
}
function requireLogin() {
    if (!loggedInUserId.value) {
        showLoginPrompt.value = true;
        return false;
    }
    return true;
}
function tooltipHtml(reason) {
    return `<div style="max-width:260px;border:1px solid #E2E8F0;border-radius:12px;background:rgba(15,23,42,0.95);color:#F8FAFC;padding:10px 12px;box-shadow:0 10px 30px rgba(15,23,42,0.25);font-size:12px;line-height:1.5;">${reason || "暂无批注原因"}</div>`;
}
function extractMarkdownSnippets(conceptName, content) {
    if (!conceptName.trim() || !content.trim())
        return [];
    const lines = content.split(/\r?\n/);
    const nc = conceptName.toLowerCase();
    const snippets = [];
    const used = new Set();
    for (let i = 0; i < lines.length; i++) {
        if (!lines[i].toLowerCase().includes(nc))
            continue;
        const s = Math.max(0, i - 1);
        const e = Math.min(lines.length - 1, i + 1);
        const k = `${s}-${e}`;
        if (used.has(k))
            continue;
        used.add(k);
        const sn = lines.slice(s, e + 1).join("\n").trim();
        if (sn)
            snippets.push(sn);
        if (snippets.length >= 3)
            break;
    }
    return snippets;
}
function buildAIExplanation(node, snippets) {
    const m = {
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
function resolveNodeIdFromEvent(event) {
    const c = [event?.data?.id, event?.target?.id, event?.target?.data?.id, event?.itemId];
    const h = c.find((v) => typeof v === "string" && v.trim().length > 0);
    if (h)
        return String(h);
    const raw = event?.target?.data ?? event?.data;
    if (raw?.nodeType && typeof raw?.id !== "undefined")
        return String(raw.id);
    return "";
}
async function handleLogin() {
    const u = userId.value.trim();
    if (!u)
        return;
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
async function loadFileList(options) {
    try {
        const r = await listUserFiles(currentUserId());
        files.value = r.files ?? [];
        fileGroups.value = r.file_groups ?? [];
    }
    catch (err) {
        if (!options?.silent) {
            throw err;
        }
    }
}
async function handleRefresh() {
    if (isRefreshing.value)
        return;
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
    }
    catch (err) {
        statusText.value = `刷新失败：${err.message}`;
    }
    finally {
        isRefreshing.value = false;
    }
}
async function handleCreateGroup() {
    const name = newGroupName.value.trim();
    if (!name || !requireLogin())
        return;
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
    }
    catch (err) {
        statusText.value = `创建文件组失败：${err.message}`;
    }
}
async function handleDeleteFile(id) {
    try {
        await deleteFile(id, currentUserId());
        if (selectedFileId.value === id)
            selectedFileId.value = "";
        menuOpen.value = "";
        await loadFileList();
    }
    catch (err) {
        statusText.value = `删除失败：${err.message}`;
    }
}
async function handleDeleteGroup(id) {
    const group = fileGroups.value.find((item) => item.id === id);
    const fileCount = files.value.filter((file) => file.file_group_id === id).length;
    const message = fileCount
        ? `确定删除文件组「${group?.name ?? id}」吗？组内 ${fileCount} 个文件也可能被删除。`
        : `确定删除空文件组「${group?.name ?? id}」吗？`;
    if (!window.confirm(message))
        return;
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
    }
    catch (err) {
        statusText.value = `删除失败：${err.message}`;
    }
}
function openRenameDialog(type, id, name) {
    renameTarget.value = { type, id, currentName: name };
    renameValue.value = name;
    menuOpen.value = "";
}
async function handleRename() {
    if (!renameTarget.value || !renameValue.value.trim())
        return;
    try {
        if (renameTarget.value.type === "file") {
            await renameFile(renameTarget.value.id, renameValue.value.trim(), currentUserId());
        }
        else {
            await renameFileGroup(renameTarget.value.id, renameValue.value.trim(), currentUserId());
        }
        renameTarget.value = null;
        await loadFileList();
    }
    catch (err) {
        statusText.value = `改名失败：${err.message}`;
    }
}
async function handleTogglePin(type, id) {
    try {
        if (type === "file")
            await togglePinFile(id, currentUserId());
        else
            await togglePinFileGroup(id, currentUserId());
        menuOpen.value = "";
        await loadFileList();
    }
    catch (err) {
        statusText.value = `操作失败：${err.message}`;
    }
}
async function handleAddFileToGroup(fileId, groupId) {
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
    }
    catch (err) {
        statusText.value = `移动文件失败：${err.message}`;
    }
}
async function selectFile(id) {
    selectedFileId.value = id;
    selectedFileGroupId.value = "";
    isSwitching.value = true;
    try {
        await Promise.all([fetchGraphByFile(id), loadConversation(id, "")]);
    }
    catch (err) {
        statusText.value = `加载失败：${err.message}`;
    }
    finally {
        isSwitching.value = false;
    }
}
async function selectFileGroup(id) {
    selectedFileGroupId.value = id;
    selectedFileId.value = "";
    isSwitching.value = true;
    try {
        await Promise.all([fetchGraphByGroup(id), loadConversation("", id)]);
    }
    catch (err) {
        statusText.value = `加载失败：${err.message}`;
    }
    finally {
        isSwitching.value = false;
    }
}
async function fetchGraphByFile(id) {
    const r = await getGraphAll({ file_id: id, user_id: currentUserId() });
    graphRawData = preprocessGraphData(r.nodes, r.edges, {
        minConfidence: 0.6,
        removeSelfLoops: true,
        keepIsolatedNodes: false,
    });
    await renderGraph(graphRawData);
    statusText.value = `图谱：${graphRawData.nodes.length} 节点 / ${graphRawData.edges.length} 连线`;
}
async function fetchGraphByGroup(id) {
    const r = await getGraphAll({ file_group_id: id, user_id: currentUserId() });
    graphRawData = preprocessGraphData(r.nodes, r.edges, {
        minConfidence: 0.6,
        removeSelfLoops: true,
        keepIsolatedNodes: false,
    });
    await renderGraph(graphRawData);
    statusText.value = `图谱：${graphRawData.nodes.length} 节点 / ${graphRawData.edges.length} 连线`;
}
async function loadConversation(fileId, fileGroupId) {
    try {
        const c = await getConversation({
            file_id: fileId || undefined,
            file_group_id: fileGroupId || undefined,
            user_id: currentUserId(),
        });
        currentConversationId.value = c.id;
        chatMessages.value = c.messages.map((m) => ({
            role: m.role,
            content: m.content,
        }));
    }
    catch {
        chatMessages.value = [];
        currentConversationId.value = "";
    }
}
async function handleClearConversation() {
    if (!currentConversationId.value)
        return;
    try {
        await deleteConversation(currentConversationId.value, currentUserId());
        chatMessages.value = [];
        currentConversationId.value = "";
    }
    catch (err) {
        statusText.value = `清空对话失败：${err.message}`;
    }
}
async function sendChatMessage(imageBase64 = "") {
    const msg = chatInput.value.trim();
    if ((!msg && !imageBase64) || isChatting.value)
        return;
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
    }).catch(() => { });
    try {
        const gn = JSON.stringify(graphRawData.nodes.map((n) => ({
            name: n.label || n.id,
            status: n.status,
            reason: n.reason,
            definition: n.data?.definition ?? "",
        })));
        const ge = JSON.stringify(graphRawData.edges.map((e) => ({
            source: e.source,
            target: e.target,
            relation: e.label,
            status: e.status,
            reason: e.reason,
        })));
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
        }).catch(() => { });
        if (r.edited_markdown) {
            markdown.value = r.edited_markdown;
            statusText.value = "AI 已修改文档，可点击生成图谱查看更新";
        }
    }
    catch (err) {
        chatMessages.value.push({ role: "ai", content: `对话出错：${err.message}` });
    }
    finally {
        isChatting.value = false;
    }
}
function handleChatImageUpload(event) {
    const input = event.target;
    const file = input?.files?.[0];
    if (!file)
        return;
    const reader = new FileReader();
    reader.onload = () => {
        const b64 = reader.result.split(",")[1];
        void sendChatMessage(b64);
    };
    reader.readAsDataURL(file);
    input.value = "";
}
async function showNodeDetail(nodeId) {
    const node = graphRawData.nodes.find((n) => n.id === nodeId);
    if (!node)
        return;
    const snippets = extractMarkdownSnippets(node.label || node.id, markdown.value);
    selectedNodeDetail.value = {
        id: node.id,
        label: node.label || node.id,
        type: node.type,
        status: node.status,
        reason: node.reason,
        snippets,
        aiExplanation: buildAIExplanation({ label: node.label || node.id, status: node.status, reason: node.reason }, snippets),
    };
    if (!markdown.value.trim())
        return;
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
    }
    catch (err) {
        if (selectedNodeDetail.value?.id === node.id) {
            selectedNodeDetail.value.aiExplanation = `讲解失败：${err.message}`;
        }
    }
    finally {
        isExplaining.value = false;
    }
}
async function initGraph() {
    if (!graphRoot.value)
        return;
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
                enable: (e) => {
                    if (e.targetType !== "node")
                        return false;
                    const s = String(e.target?.data?.status ?? "");
                    return s === "error" || s === "supplement";
                },
                getContent: (e) => tooltipHtml(String(e.target?.data?.reason ?? "")),
            },
        ],
    });
    await graph.render();
    graph.on("node:click", (e) => {
        const id = resolveNodeIdFromEvent(e);
        if (!id)
            return;
        if (isLightRAGMode.value && !expandedNodes.value.has(id)) {
            void expandNodeNeighbors(id);
        }
        else if (isFocusMode.value) {
            void focusNode(id);
            void showNodeDetail(id);
        }
        else {
            void showNodeDetail(id);
        }
    });
    graph.on("node:pointerenter", (e) => {
        const id = resolveNodeIdFromEvent(e);
        if (!id || !graph)
            return;
        const edgeIds = graphRawData.edges
            .filter((edge) => edge.source === id || edge.target === id)
            .map((edge) => edge.id || `${edge.source}-${edge.target}-${edge.label}`);
        graph.setElementState(id, "highlight");
        edgeIds.forEach((edgeId) => graph?.setElementState(edgeId, "highlight"));
    });
    graph.on("node:pointerleave", () => {
        graph?.getData().nodes.forEach((node) => graph?.setElementState(node.id, []));
        graph?.getData().edges.forEach((edge) => graph?.setElementState(edge.id, []));
    });
    graph.on("canvas:click", () => {
        selectedNodeDetail.value = null;
    });
}
function graphDataForDisplay(data) {
    if (graphStatusFilter.value === "all")
        return data;
    const nodes = data.nodes.filter((node) => node.status === graphStatusFilter.value);
    const ids = new Set(nodes.map((node) => node.id));
    return { nodes, edges: data.edges.filter((edge) => ids.has(edge.source) && ids.has(edge.target)) };
}
function localFocusData(nodeId, depth = 2) {
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
function preferredLayout() {
    if (graphLayoutMode.value !== "auto")
        return graphLayoutMode.value;
    return isNavigating.value || Boolean(selectedFileId.value) ? "dagre" : "force";
}
function prepareHierarchyData(data) {
    const dependencyPattern = /PREREQUISITE|DEPENDS|REQUIRES|PRECEDES|PARENT|IS_A|PART_OF/i;
    const hierarchyEdges = data.edges.filter((edge) => dependencyPattern.test(edge.label || ""));
    if (hierarchyEdges.length === 0)
        return data;
    const supportingEdges = data.edges.filter((edge) => edge.status === "error" || edge.status === "supplement" || dependencyPattern.test(edge.label || ""));
    const connected = new Set();
    supportingEdges.forEach((edge) => {
        connected.add(edge.source);
        connected.add(edge.target);
    });
    return {
        nodes: data.nodes.filter((node) => connected.has(node.id)),
        edges: supportingEdges,
    };
}
async function renderGraph(data, focusMode = false, pathData = [], layoutType = "force") {
    if (!graph)
        return;
    const selectedLayout = graphLayoutMode.value === "auto" ? layoutType : graphLayoutMode.value;
    activeLayout.value = selectedLayout;
    const visibleData = graphDataForDisplay(selectedLayout === "dagre" ? prepareHierarchyData(data) : data);
    const focusSet = focusMode ? buildFocusSet(pathData) : undefined;
    const styled = buildStyledGraph(visibleData, focusSet);
    graph.setLayout(styled.nodes.some((n) => n.x !== undefined)
        ? { type: "preset", padding: 50 }
        : getLayoutConfig(selectedLayout));
    graph.setData(styled);
    await graph.render();
}
async function expandNodeNeighbors(nodeId) {
    if (!graph)
        return;
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
        graph.addData(s);
        await graph.render();
        statusText.value = `已展开 ${nn.length} 节点 ${ne.length} 边`;
    }
    catch (e) {
        statusText.value = `展开失败：${e.message}`;
    }
}
function extractCoreNodes(data, n) {
    const deg = new Map();
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
function pushDiagnosisSummary(result) {
    const diag = result.diagnose_result;
    const nodes = diag?.nodes ?? [];
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
        if (corrects.length > 5)
            summary += `\n... 等 ${corrects.length - 5} 个`;
        summary += "\n\n";
    }
    if (errors.length === 0 && supplements.length === 0) {
        summary += `笔记质量很好，未发现错误或知识缺口。\n\n`;
    }
    summary += `你可以继续提问，让我帮你修正错误或补充缺失知识点。`;
    chatMessages.value.push({ role: "ai", content: summary });
}
async function handleUpload() {
    if (!canGenerate.value)
        return;
    isLoading.value = true;
    statusText.value = "AI 诊断中（NER → 校验 → 补全）…";
    try {
        const r = await uploadNoteLangChain({
            markdown: markdown.value,
            user_id: currentUserId(),
            file_id: selectedFileId.value || undefined,
            file_group_id: selectedFileGroupId.value || undefined,
        });
        if (r.file_id && !selectedFileId.value)
            selectedFileId.value = r.file_id;
        await loadFileList();
        await fetchAllGraph();
        pushDiagnosisSummary(r);
        isNavigating.value = false;
        selectedNodeDetail.value = null;
    }
    catch (err) {
        statusText.value = `生成失败：${err.message}`;
    }
    finally {
        isLoading.value = false;
    }
}
async function handleImportMarkdownFile(event) {
    const input = event.target;
    const file = input?.files?.[0];
    if (!file)
        return;
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
    }
    catch (err) {
        statusText.value = `读取失败：${err.message}`;
        input.value = "";
    }
}
async function handleLeftUploadFile(event) {
    await handleImportMarkdownFile(event);
    if (markdown.value)
        await handleUpload();
}
async function handleLeftUploadFileGroup(event) {
    const input = event.target;
    const fls = input?.files;
    if (!fls || fls.length === 0)
        return;
    if (!requireLogin()) {
        input.value = "";
        return;
    }
    isLoading.value = true;
    statusText.value = `创建文件组并处理 ${fls.length} 个文件…`;
    try {
        const gn = `文件组_${new Date().toLocaleDateString()}`;
        const gr = await createFileGroup(gn, currentUserId());
        const gid = gr.group_id;
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
    }
    catch (err) {
        statusText.value = `创建文件组失败：${err.message}`;
    }
    finally {
        isLoading.value = false;
        input.value = "";
    }
}
async function handlePathNavigate() {
    if (!canNavigate.value)
        return;
    isNavigating.value = true;
    statusText.value = "计算逆向学习路径…";
    try {
        const r = await getGraphPath(concept.value.trim(), currentUserId(), maxDepth.value);
        const hasRelated = r.all_related && r.all_related.nodes && r.all_related.nodes.length > 0;
        await renderGraph(hasRelated ? r.all_related : graphRawData, true, hasRelated ? [r.all_related] : r.paths, "dagre");
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
    }
    catch (err) {
        statusText.value = `路径查询失败：${err.message}`;
    }
}
async function resetFocus() {
    isFocusMode.value = false;
    focusedNodeId.value = "";
    await renderGraph(graphRawData, false, [], "force");
    isNavigating.value = false;
    statusText.value = "已退出专注模式";
}
async function focusNode(nodeId) {
    const focused = localFocusData(nodeId);
    isFocusMode.value = true;
    focusedNodeId.value = nodeId;
    await renderGraph(graphRawData, true, [focused], "force");
    await graph?.focusElement?.(nodeId, { animation: { duration: 500 } });
    statusText.value = `专注模式：${focused.nodes.length} 个相关知识点`;
}
async function searchGraph() {
    const keyword = graphSearch.value.trim().toLowerCase();
    if (!keyword)
        return;
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
    await renderGraph(graphRawData, isFocusMode.value, isFocusMode.value && focusedNodeId.value ? [localFocusData(focusedNodeId.value)] : [], preferredLayout());
    statusText.value = activeLayout.value === "dagre" ? "层级布局：前置知识从左向右排列" : "关系网络：按关联强度排列";
}
function fitGraph() {
    void graph?.fitView?.({ when: "always", direction: "both" });
}
function zoomGraph(ratio) {
    void graph?.zoomBy?.(ratio);
}
onMounted(async () => {
    await initGraph();
    await loadFileList({ silent: true });
    try {
        await fetchAllGraph();
    }
    catch {
        statusText.value = "图谱待生成";
    }
    if (graphRoot.value && graph) {
        resizeObserver = new ResizeObserver((e) => {
            const r = e[0];
            if (r && graph)
                graph.resize(r.contentRect.width, r.contentRect.height);
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
debugger; /* PartiallyEnd: #3632/scriptSetup.vue */
const __VLS_ctx = {};
let __VLS_components;
let __VLS_directives;
__VLS_asFunctionalElement(__VLS_intrinsicElements.main, __VLS_intrinsicElements.main)({
    ...{ class: "flex h-screen w-full overflow-hidden bg-[#F8FAFC]" },
});
/** @type {[typeof FileSidebar, ]} */ ;
// @ts-ignore
const __VLS_0 = __VLS_asFunctionalComponent(FileSidebar, new FileSidebar({
    ...{ 'onLoginRequired': {} },
    ...{ 'onUploadFile': {} },
    ...{ 'onUploadFileGroup': {} },
    ...{ 'onCreateGroup': {} },
    ...{ 'onSelectFile': {} },
    ...{ 'onSelectFileGroup': {} },
    ...{ 'onTogglePin': {} },
    ...{ 'onRename': {} },
    ...{ 'onDeleteFile': {} },
    ...{ 'onDeleteGroup': {} },
    ...{ 'onAddToGroup': {} },
    ...{ 'onRefresh': {} },
    newGroupName: (__VLS_ctx.newGroupName),
    menuOpen: (__VLS_ctx.menuOpen),
    width: (__VLS_ctx.leftWidth),
    loggedInUserId: (__VLS_ctx.loggedInUserId),
    files: (__VLS_ctx.files),
    fileGroups: (__VLS_ctx.fileGroups),
    selectedFileId: (__VLS_ctx.selectedFileId),
    selectedFileGroupId: (__VLS_ctx.selectedFileGroupId),
    isRefreshing: (__VLS_ctx.isRefreshing),
}));
const __VLS_1 = __VLS_0({
    ...{ 'onLoginRequired': {} },
    ...{ 'onUploadFile': {} },
    ...{ 'onUploadFileGroup': {} },
    ...{ 'onCreateGroup': {} },
    ...{ 'onSelectFile': {} },
    ...{ 'onSelectFileGroup': {} },
    ...{ 'onTogglePin': {} },
    ...{ 'onRename': {} },
    ...{ 'onDeleteFile': {} },
    ...{ 'onDeleteGroup': {} },
    ...{ 'onAddToGroup': {} },
    ...{ 'onRefresh': {} },
    newGroupName: (__VLS_ctx.newGroupName),
    menuOpen: (__VLS_ctx.menuOpen),
    width: (__VLS_ctx.leftWidth),
    loggedInUserId: (__VLS_ctx.loggedInUserId),
    files: (__VLS_ctx.files),
    fileGroups: (__VLS_ctx.fileGroups),
    selectedFileId: (__VLS_ctx.selectedFileId),
    selectedFileGroupId: (__VLS_ctx.selectedFileGroupId),
    isRefreshing: (__VLS_ctx.isRefreshing),
}, ...__VLS_functionalComponentArgsRest(__VLS_0));
let __VLS_3;
let __VLS_4;
let __VLS_5;
const __VLS_6 = {
    onLoginRequired: (...[$event]) => {
        __VLS_ctx.showLoginPrompt = true;
    }
};
const __VLS_7 = {
    onUploadFile: (__VLS_ctx.handleLeftUploadFile)
};
const __VLS_8 = {
    onUploadFileGroup: (__VLS_ctx.handleLeftUploadFileGroup)
};
const __VLS_9 = {
    onCreateGroup: (__VLS_ctx.handleCreateGroup)
};
const __VLS_10 = {
    onSelectFile: (__VLS_ctx.selectFile)
};
const __VLS_11 = {
    onSelectFileGroup: (__VLS_ctx.selectFileGroup)
};
const __VLS_12 = {
    onTogglePin: (__VLS_ctx.handleTogglePin)
};
const __VLS_13 = {
    onRename: (__VLS_ctx.openRenameDialog)
};
const __VLS_14 = {
    onDeleteFile: (__VLS_ctx.handleDeleteFile)
};
const __VLS_15 = {
    onDeleteGroup: (__VLS_ctx.handleDeleteGroup)
};
const __VLS_16 = {
    onAddToGroup: ((id) => (__VLS_ctx.addFileToGroupTarget = id))
};
const __VLS_17 = {
    onRefresh: (__VLS_ctx.handleRefresh)
};
var __VLS_2;
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
    ...{ onMousedown: (...[$event]) => {
            __VLS_ctx.onDividerMousedown('left', $event);
        } },
    ...{ class: "group relative w-1.5 shrink-0 cursor-col-resize bg-transparent transition-colors hover:bg-indigo-300 active:bg-indigo-400" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div)({
    ...{ class: "absolute inset-y-0 -left-1 -right-1" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div)({
    ...{ class: "absolute left-1/2 top-1/2 h-8 w-1 -translate-x-1/2 -translate-y-1/2 rounded-full bg-slate-300 transition-colors group-hover:bg-indigo-400" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.section, __VLS_intrinsicElements.section)({
    ...{ class: "flex min-w-0 flex-1 flex-col overflow-hidden" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.header, __VLS_intrinsicElements.header)({
    ...{ class: "flex items-center gap-3 border-b border-gray-200 bg-white px-5 py-3" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
    ...{ class: "min-w-0" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.h1, __VLS_intrinsicElements.h1)({
    ...{ class: "text-lg font-semibold text-slate-900" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
    ...{ class: "truncate text-[13px] text-slate-500" },
});
(__VLS_ctx.isSwitching ? "加载中…" : __VLS_ctx.statusText);
__VLS_asFunctionalElement(__VLS_intrinsicElements.div)({
    ...{ class: "flex-1" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
    ...{ onClick: (...[$event]) => {
            __VLS_ctx.isLightRAGMode = !__VLS_ctx.isLightRAGMode;
            __VLS_ctx.fetchAllGraph();
        } },
    type: "button",
    ...{ class: "h-11 shrink-0 rounded-xl border border-gray-200 bg-white px-4 text-sm font-medium text-slate-700 transition duration-200 hover:bg-gray-100" },
});
(__VLS_ctx.isLightRAGMode ? "渐进式展开" : "显示全图");
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
    ...{ class: "relative m-3 min-h-0 flex-1 overflow-hidden rounded-[18px] border border-gray-200 bg-white shadow-sm" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div)({
    ref: "graphRoot",
    ...{ class: "absolute inset-0" },
});
/** @type {typeof __VLS_ctx.graphRoot} */ ;
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
    ...{ class: "absolute left-4 top-4 z-20 flex flex-wrap items-center gap-2 rounded-2xl border border-slate-200 bg-white/95 p-2 shadow-sm backdrop-blur" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
    ...{ class: "flex h-9 items-center overflow-hidden rounded-xl border border-slate-200 bg-white" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.input)({
    ...{ onKeyup: (__VLS_ctx.searchGraph) },
    ...{ class: "w-40 px-3 text-sm outline-none" },
    placeholder: "搜索概念…",
});
(__VLS_ctx.graphSearch);
__VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
    ...{ onClick: (__VLS_ctx.searchGraph) },
    ...{ class: "h-full border-l border-slate-200 px-3 text-xs font-medium text-indigo-600 hover:bg-indigo-50" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.select, __VLS_intrinsicElements.select)({
    ...{ onChange: (__VLS_ctx.applyStatusFilter) },
    value: (__VLS_ctx.graphStatusFilter),
    ...{ class: "h-9 rounded-xl border border-slate-200 bg-white px-2 text-xs text-slate-600 outline-none" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.option, __VLS_intrinsicElements.option)({
    value: "all",
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.option, __VLS_intrinsicElements.option)({
    value: "correct",
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.option, __VLS_intrinsicElements.option)({
    value: "error",
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.option, __VLS_intrinsicElements.option)({
    value: "supplement",
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.select, __VLS_intrinsicElements.select)({
    ...{ onChange: (__VLS_ctx.applyLayoutMode) },
    value: (__VLS_ctx.graphLayoutMode),
    ...{ class: "h-9 rounded-xl border border-slate-200 bg-white px-2 text-xs text-slate-600 outline-none" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.option, __VLS_intrinsicElements.option)({
    value: "auto",
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.option, __VLS_intrinsicElements.option)({
    value: "dagre",
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.option, __VLS_intrinsicElements.option)({
    value: "force",
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
    ...{ onClick: (...[$event]) => {
            __VLS_ctx.isFocusMode ? __VLS_ctx.resetFocus() : (__VLS_ctx.isFocusMode = true);
        } },
    ...{ class: "h-9 rounded-xl px-3 text-xs font-medium transition" },
    ...{ class: (__VLS_ctx.isFocusMode ? 'bg-indigo-600 text-white' : 'bg-slate-100 text-slate-600 hover:bg-slate-200') },
});
(__VLS_ctx.isFocusMode ? "退出专注" : "专注模式");
__VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
    ...{ onClick: (__VLS_ctx.fitGraph) },
    ...{ class: "h-9 rounded-xl bg-slate-100 px-3 text-xs text-slate-600 hover:bg-slate-200" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
    ...{ onClick: (...[$event]) => {
            __VLS_ctx.zoomGraph(1.2);
        } },
    ...{ class: "h-9 w-9 rounded-xl bg-slate-100 text-sm text-slate-600 hover:bg-slate-200" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
    ...{ onClick: (...[$event]) => {
            __VLS_ctx.zoomGraph(0.8);
        } },
    ...{ class: "h-9 w-9 rounded-xl bg-slate-100 text-sm text-slate-600 hover:bg-slate-200" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
    ...{ class: "absolute bottom-4 right-4 z-10 rounded-xl border border-slate-200 bg-white/90 px-3 py-2 text-[11px] text-slate-500 shadow-sm backdrop-blur" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.span, __VLS_intrinsicElements.span)({
    ...{ class: "mr-3" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.i)({
    ...{ class: "mr-1 inline-block h-2.5 w-2.5 rounded-full bg-blue-500" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.span, __VLS_intrinsicElements.span)({
    ...{ class: "mr-3" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.i)({
    ...{ class: "mr-1 inline-block h-2.5 w-2.5 rounded-full bg-red-500" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.span, __VLS_intrinsicElements.span)({});
__VLS_asFunctionalElement(__VLS_intrinsicElements.i)({
    ...{ class: "mr-1 inline-block h-2.5 w-2.5 rounded-full border border-dashed border-violet-600 bg-violet-100" },
});
if (__VLS_ctx.selectedNodeDetail) {
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ class: "absolute bottom-4 left-4 right-4 max-w-md rounded-[18px] border border-gray-200 bg-white/95 p-4 shadow-md backdrop-blur" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ class: "mb-2 flex items-start justify-between gap-3" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({});
    __VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
        ...{ class: "text-base font-semibold text-slate-900" },
    });
    (__VLS_ctx.selectedNodeDetail.label);
    __VLS_asFunctionalElement(__VLS_intrinsicElements.span, __VLS_intrinsicElements.span)({
        ...{ class: "mt-1 inline-block rounded-lg px-2 py-0.5 text-[13px] font-medium" },
        ...{ class: (__VLS_ctx.selectedNodeDetail.status === 'error'
                ? 'bg-red-50 text-red-600'
                : __VLS_ctx.selectedNodeDetail.status === 'supplement'
                    ? 'bg-violet-50 text-violet-700'
                    : 'bg-slate-100 text-slate-600') },
    });
    (__VLS_ctx.selectedNodeDetail.status || "unknown");
    __VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.selectedNodeDetail))
                    return;
                __VLS_ctx.selectedNodeDetail = null;
            } },
        type: "button",
        ...{ class: "text-slate-400 transition hover:text-slate-600" },
    });
    if (__VLS_ctx.selectedNodeDetail.reason) {
        __VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
            ...{ class: "mb-2 text-[13px] text-indigo-600" },
        });
        (__VLS_ctx.selectedNodeDetail.reason);
    }
    if (__VLS_ctx.isExplaining) {
        __VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
            ...{ class: "text-[13px] text-slate-400" },
        });
    }
    else {
        __VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
            ...{ class: "whitespace-pre-wrap text-sm leading-relaxed text-slate-600" },
        });
        (__VLS_ctx.selectedNodeDetail.aiExplanation);
    }
}
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
    ...{ onMousedown: (...[$event]) => {
            __VLS_ctx.onDividerMousedown('right', $event);
        } },
    ...{ class: "group relative w-1.5 shrink-0 cursor-col-resize bg-transparent transition-colors hover:bg-violet-300 active:bg-violet-400" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div)({
    ...{ class: "absolute inset-y-0 -left-1 -right-1" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div)({
    ...{ class: "absolute left-1/2 top-1/2 h-8 w-1 -translate-x-1/2 -translate-y-1/2 rounded-full bg-slate-300 transition-colors group-hover:bg-violet-400" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.aside, __VLS_intrinsicElements.aside)({
    ...{ style: ({ width: `${__VLS_ctx.rightWidth}px` }) },
    ...{ class: "flex shrink-0 flex-col gap-3 overflow-hidden border-l border-gray-200 bg-[#F8FAFC] p-3" },
});
/** @type {[typeof ImportPanel, ]} */ ;
// @ts-ignore
const __VLS_18 = __VLS_asFunctionalComponent(ImportPanel, new ImportPanel({
    ...{ 'onLogin': {} },
    ...{ 'onImportFile': {} },
    ...{ 'onGenerate': {} },
    markdown: (__VLS_ctx.markdown),
    userId: (__VLS_ctx.userId),
    importedFileName: (__VLS_ctx.importedFileName),
    loggedInUserId: (__VLS_ctx.loggedInUserId),
    isLoading: (__VLS_ctx.isLoading),
    canGenerate: (__VLS_ctx.canGenerate),
}));
const __VLS_19 = __VLS_18({
    ...{ 'onLogin': {} },
    ...{ 'onImportFile': {} },
    ...{ 'onGenerate': {} },
    markdown: (__VLS_ctx.markdown),
    userId: (__VLS_ctx.userId),
    importedFileName: (__VLS_ctx.importedFileName),
    loggedInUserId: (__VLS_ctx.loggedInUserId),
    isLoading: (__VLS_ctx.isLoading),
    canGenerate: (__VLS_ctx.canGenerate),
}, ...__VLS_functionalComponentArgsRest(__VLS_18));
let __VLS_21;
let __VLS_22;
let __VLS_23;
const __VLS_24 = {
    onLogin: (__VLS_ctx.handleLogin)
};
const __VLS_25 = {
    onImportFile: (__VLS_ctx.handleImportMarkdownFile)
};
const __VLS_26 = {
    onGenerate: (__VLS_ctx.handleUpload)
};
var __VLS_20;
/** @type {[typeof LearningNavPanel, ]} */ ;
// @ts-ignore
const __VLS_27 = __VLS_asFunctionalComponent(LearningNavPanel, new LearningNavPanel({
    ...{ 'onNavigate': {} },
    ...{ 'onReset': {} },
    concept: (__VLS_ctx.concept),
    maxDepth: (__VLS_ctx.maxDepth),
    canNavigate: (__VLS_ctx.canNavigate),
    isLoading: (__VLS_ctx.isLoading),
    isNavigating: (__VLS_ctx.isNavigating),
}));
const __VLS_28 = __VLS_27({
    ...{ 'onNavigate': {} },
    ...{ 'onReset': {} },
    concept: (__VLS_ctx.concept),
    maxDepth: (__VLS_ctx.maxDepth),
    canNavigate: (__VLS_ctx.canNavigate),
    isLoading: (__VLS_ctx.isLoading),
    isNavigating: (__VLS_ctx.isNavigating),
}, ...__VLS_functionalComponentArgsRest(__VLS_27));
let __VLS_30;
let __VLS_31;
let __VLS_32;
const __VLS_33 = {
    onNavigate: (__VLS_ctx.handlePathNavigate)
};
const __VLS_34 = {
    onReset: (__VLS_ctx.resetFocus)
};
var __VLS_29;
/** @type {[typeof AiChatPanel, ]} */ ;
// @ts-ignore
const __VLS_35 = __VLS_asFunctionalComponent(AiChatPanel, new AiChatPanel({
    ...{ 'onSend': {} },
    ...{ 'onClear': {} },
    ...{ 'onUploadImage': {} },
    chatInput: (__VLS_ctx.chatInput),
    messages: (__VLS_ctx.chatMessages),
    isChatting: (__VLS_ctx.isChatting),
    hasConversation: (!!__VLS_ctx.currentConversationId),
    renderMarkdown: (__VLS_ctx.renderMarkdown),
}));
const __VLS_36 = __VLS_35({
    ...{ 'onSend': {} },
    ...{ 'onClear': {} },
    ...{ 'onUploadImage': {} },
    chatInput: (__VLS_ctx.chatInput),
    messages: (__VLS_ctx.chatMessages),
    isChatting: (__VLS_ctx.isChatting),
    hasConversation: (!!__VLS_ctx.currentConversationId),
    renderMarkdown: (__VLS_ctx.renderMarkdown),
}, ...__VLS_functionalComponentArgsRest(__VLS_35));
let __VLS_38;
let __VLS_39;
let __VLS_40;
const __VLS_41 = {
    onSend: (...[$event]) => {
        __VLS_ctx.sendChatMessage();
    }
};
const __VLS_42 = {
    onClear: (__VLS_ctx.handleClearConversation)
};
const __VLS_43 = {
    onUploadImage: (__VLS_ctx.handleChatImageUpload)
};
var __VLS_37;
if (__VLS_ctx.showLoginPrompt) {
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.showLoginPrompt))
                    return;
                __VLS_ctx.showLoginPrompt = false;
            } },
        ...{ class: "fixed inset-0 z-50 flex items-center justify-center bg-black/20" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ class: "w-[320px] rounded-[20px] border border-gray-200 bg-white p-6 text-center shadow-lg" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
        ...{ class: "text-base font-semibold text-slate-800" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
        ...{ class: "mt-2 text-sm text-slate-500" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.showLoginPrompt))
                    return;
                __VLS_ctx.showLoginPrompt = false;
            } },
        type: "button",
        ...{ class: "mt-4 h-11 w-full rounded-xl bg-indigo-600 text-sm font-medium text-white transition hover:bg-indigo-500" },
    });
}
if (__VLS_ctx.addFileToGroupTarget) {
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.addFileToGroupTarget))
                    return;
                __VLS_ctx.addFileToGroupTarget = '';
            } },
        ...{ class: "fixed inset-0 z-50 flex items-center justify-center bg-black/20" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ class: "w-[280px] rounded-[20px] border border-gray-200 bg-white p-4 shadow-lg" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
        ...{ class: "mb-3 text-sm font-semibold text-slate-800" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ class: "mb-3 max-h-[180px] space-y-1 overflow-y-auto" },
    });
    for (const [g] of __VLS_getVForSourceType((__VLS_ctx.fileGroups))) {
        __VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
            ...{ onClick: (...[$event]) => {
                    if (!(__VLS_ctx.addFileToGroupTarget))
                        return;
                    __VLS_ctx.handleAddFileToGroup(__VLS_ctx.addFileToGroupTarget, g.id);
                } },
            key: (g.id),
            type: "button",
            disabled: (__VLS_ctx.files.find((f) => f.id === __VLS_ctx.addFileToGroupTarget)?.file_group_id === g.id),
            ...{ class: "flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm text-slate-700 hover:bg-violet-50 disabled:cursor-default disabled:bg-slate-50 disabled:text-slate-400" },
        });
        __VLS_asFunctionalElement(__VLS_intrinsicElements.span, __VLS_intrinsicElements.span)({
            ...{ class: "truncate" },
        });
        (g.name);
        if (__VLS_ctx.files.find((f) => f.id === __VLS_ctx.addFileToGroupTarget)?.file_group_id === g.id) {
            __VLS_asFunctionalElement(__VLS_intrinsicElements.span, __VLS_intrinsicElements.span)({
                ...{ class: "text-[12px]" },
            });
        }
    }
    if (__VLS_ctx.fileGroups.length === 0) {
        __VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
            ...{ class: "px-2 text-sm text-slate-400" },
        });
    }
    __VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.addFileToGroupTarget))
                    return;
                __VLS_ctx.addFileToGroupTarget = '';
            } },
        type: "button",
        ...{ class: "h-11 w-full rounded-xl bg-slate-100 text-sm text-slate-600 hover:bg-slate-200" },
    });
}
if (__VLS_ctx.renameTarget) {
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.renameTarget))
                    return;
                __VLS_ctx.renameTarget = null;
            } },
        ...{ class: "fixed inset-0 z-50 flex items-center justify-center bg-black/20" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ class: "w-[280px] rounded-[20px] border border-gray-200 bg-white p-4 shadow-lg" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
        ...{ class: "mb-3 text-sm font-semibold text-slate-800" },
    });
    (__VLS_ctx.renameTarget.type === "file" ? "重命名文件" : "重命名文件组");
    __VLS_asFunctionalElement(__VLS_intrinsicElements.input)({
        ...{ onKeyup: (__VLS_ctx.handleRename) },
        ...{ class: "mb-3 h-11 w-full rounded-xl border border-gray-200 px-3 text-sm outline-none focus:border-indigo-500" },
    });
    (__VLS_ctx.renameValue);
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ class: "flex gap-2" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.renameTarget))
                    return;
                __VLS_ctx.renameTarget = null;
            } },
        type: "button",
        ...{ class: "h-11 flex-1 rounded-xl bg-slate-100 text-sm text-slate-600 hover:bg-slate-200" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
        ...{ onClick: (__VLS_ctx.handleRename) },
        type: "button",
        ...{ class: "h-11 flex-1 rounded-xl bg-indigo-600 text-sm text-white hover:bg-indigo-500" },
    });
}
if (__VLS_ctx.isLoading) {
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ class: "fixed inset-0 z-50 flex items-center justify-center bg-slate-950/30 backdrop-blur-[2px]" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ class: "rounded-[20px] border border-white/10 bg-slate-900/90 px-8 py-6 text-center text-white shadow-lg" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div)({
        ...{ class: "mx-auto h-10 w-10 animate-spin rounded-full border-2 border-white/20 border-t-white" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
        ...{ class: "mt-3 text-sm" },
    });
}
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['h-screen']} */ ;
/** @type {__VLS_StyleScopedClasses['w-full']} */ ;
/** @type {__VLS_StyleScopedClasses['overflow-hidden']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-[#F8FAFC]']} */ ;
/** @type {__VLS_StyleScopedClasses['group']} */ ;
/** @type {__VLS_StyleScopedClasses['relative']} */ ;
/** @type {__VLS_StyleScopedClasses['w-1.5']} */ ;
/** @type {__VLS_StyleScopedClasses['shrink-0']} */ ;
/** @type {__VLS_StyleScopedClasses['cursor-col-resize']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-transparent']} */ ;
/** @type {__VLS_StyleScopedClasses['transition-colors']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-indigo-300']} */ ;
/** @type {__VLS_StyleScopedClasses['active:bg-indigo-400']} */ ;
/** @type {__VLS_StyleScopedClasses['absolute']} */ ;
/** @type {__VLS_StyleScopedClasses['inset-y-0']} */ ;
/** @type {__VLS_StyleScopedClasses['-left-1']} */ ;
/** @type {__VLS_StyleScopedClasses['-right-1']} */ ;
/** @type {__VLS_StyleScopedClasses['absolute']} */ ;
/** @type {__VLS_StyleScopedClasses['left-1/2']} */ ;
/** @type {__VLS_StyleScopedClasses['top-1/2']} */ ;
/** @type {__VLS_StyleScopedClasses['h-8']} */ ;
/** @type {__VLS_StyleScopedClasses['w-1']} */ ;
/** @type {__VLS_StyleScopedClasses['-translate-x-1/2']} */ ;
/** @type {__VLS_StyleScopedClasses['-translate-y-1/2']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-full']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-300']} */ ;
/** @type {__VLS_StyleScopedClasses['transition-colors']} */ ;
/** @type {__VLS_StyleScopedClasses['group-hover:bg-indigo-400']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['min-w-0']} */ ;
/** @type {__VLS_StyleScopedClasses['flex-1']} */ ;
/** @type {__VLS_StyleScopedClasses['flex-col']} */ ;
/** @type {__VLS_StyleScopedClasses['overflow-hidden']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['items-center']} */ ;
/** @type {__VLS_StyleScopedClasses['gap-3']} */ ;
/** @type {__VLS_StyleScopedClasses['border-b']} */ ;
/** @type {__VLS_StyleScopedClasses['border-gray-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['px-5']} */ ;
/** @type {__VLS_StyleScopedClasses['py-3']} */ ;
/** @type {__VLS_StyleScopedClasses['min-w-0']} */ ;
/** @type {__VLS_StyleScopedClasses['text-lg']} */ ;
/** @type {__VLS_StyleScopedClasses['font-semibold']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-900']} */ ;
/** @type {__VLS_StyleScopedClasses['truncate']} */ ;
/** @type {__VLS_StyleScopedClasses['text-[13px]']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-500']} */ ;
/** @type {__VLS_StyleScopedClasses['flex-1']} */ ;
/** @type {__VLS_StyleScopedClasses['h-11']} */ ;
/** @type {__VLS_StyleScopedClasses['shrink-0']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-gray-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['px-4']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['font-medium']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-700']} */ ;
/** @type {__VLS_StyleScopedClasses['transition']} */ ;
/** @type {__VLS_StyleScopedClasses['duration-200']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-gray-100']} */ ;
/** @type {__VLS_StyleScopedClasses['relative']} */ ;
/** @type {__VLS_StyleScopedClasses['m-3']} */ ;
/** @type {__VLS_StyleScopedClasses['min-h-0']} */ ;
/** @type {__VLS_StyleScopedClasses['flex-1']} */ ;
/** @type {__VLS_StyleScopedClasses['overflow-hidden']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-[18px]']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-gray-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['shadow-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['absolute']} */ ;
/** @type {__VLS_StyleScopedClasses['inset-0']} */ ;
/** @type {__VLS_StyleScopedClasses['absolute']} */ ;
/** @type {__VLS_StyleScopedClasses['left-4']} */ ;
/** @type {__VLS_StyleScopedClasses['top-4']} */ ;
/** @type {__VLS_StyleScopedClasses['z-20']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['flex-wrap']} */ ;
/** @type {__VLS_StyleScopedClasses['items-center']} */ ;
/** @type {__VLS_StyleScopedClasses['gap-2']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-2xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white/95']} */ ;
/** @type {__VLS_StyleScopedClasses['p-2']} */ ;
/** @type {__VLS_StyleScopedClasses['shadow-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['backdrop-blur']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['h-9']} */ ;
/** @type {__VLS_StyleScopedClasses['items-center']} */ ;
/** @type {__VLS_StyleScopedClasses['overflow-hidden']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['w-40']} */ ;
/** @type {__VLS_StyleScopedClasses['px-3']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['outline-none']} */ ;
/** @type {__VLS_StyleScopedClasses['h-full']} */ ;
/** @type {__VLS_StyleScopedClasses['border-l']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['px-3']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['font-medium']} */ ;
/** @type {__VLS_StyleScopedClasses['text-indigo-600']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-indigo-50']} */ ;
/** @type {__VLS_StyleScopedClasses['h-9']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['px-2']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-600']} */ ;
/** @type {__VLS_StyleScopedClasses['outline-none']} */ ;
/** @type {__VLS_StyleScopedClasses['h-9']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['px-2']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-600']} */ ;
/** @type {__VLS_StyleScopedClasses['outline-none']} */ ;
/** @type {__VLS_StyleScopedClasses['h-9']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['px-3']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['font-medium']} */ ;
/** @type {__VLS_StyleScopedClasses['transition']} */ ;
/** @type {__VLS_StyleScopedClasses['h-9']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-100']} */ ;
/** @type {__VLS_StyleScopedClasses['px-3']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-600']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['h-9']} */ ;
/** @type {__VLS_StyleScopedClasses['w-9']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-100']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-600']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['h-9']} */ ;
/** @type {__VLS_StyleScopedClasses['w-9']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-100']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-600']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['absolute']} */ ;
/** @type {__VLS_StyleScopedClasses['bottom-4']} */ ;
/** @type {__VLS_StyleScopedClasses['right-4']} */ ;
/** @type {__VLS_StyleScopedClasses['z-10']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white/90']} */ ;
/** @type {__VLS_StyleScopedClasses['px-3']} */ ;
/** @type {__VLS_StyleScopedClasses['py-2']} */ ;
/** @type {__VLS_StyleScopedClasses['text-[11px]']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-500']} */ ;
/** @type {__VLS_StyleScopedClasses['shadow-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['backdrop-blur']} */ ;
/** @type {__VLS_StyleScopedClasses['mr-3']} */ ;
/** @type {__VLS_StyleScopedClasses['mr-1']} */ ;
/** @type {__VLS_StyleScopedClasses['inline-block']} */ ;
/** @type {__VLS_StyleScopedClasses['h-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['w-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-full']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-blue-500']} */ ;
/** @type {__VLS_StyleScopedClasses['mr-3']} */ ;
/** @type {__VLS_StyleScopedClasses['mr-1']} */ ;
/** @type {__VLS_StyleScopedClasses['inline-block']} */ ;
/** @type {__VLS_StyleScopedClasses['h-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['w-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-full']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-red-500']} */ ;
/** @type {__VLS_StyleScopedClasses['mr-1']} */ ;
/** @type {__VLS_StyleScopedClasses['inline-block']} */ ;
/** @type {__VLS_StyleScopedClasses['h-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['w-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-full']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-dashed']} */ ;
/** @type {__VLS_StyleScopedClasses['border-violet-600']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-violet-100']} */ ;
/** @type {__VLS_StyleScopedClasses['absolute']} */ ;
/** @type {__VLS_StyleScopedClasses['bottom-4']} */ ;
/** @type {__VLS_StyleScopedClasses['left-4']} */ ;
/** @type {__VLS_StyleScopedClasses['right-4']} */ ;
/** @type {__VLS_StyleScopedClasses['max-w-md']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-[18px]']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-gray-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white/95']} */ ;
/** @type {__VLS_StyleScopedClasses['p-4']} */ ;
/** @type {__VLS_StyleScopedClasses['shadow-md']} */ ;
/** @type {__VLS_StyleScopedClasses['backdrop-blur']} */ ;
/** @type {__VLS_StyleScopedClasses['mb-2']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['items-start']} */ ;
/** @type {__VLS_StyleScopedClasses['justify-between']} */ ;
/** @type {__VLS_StyleScopedClasses['gap-3']} */ ;
/** @type {__VLS_StyleScopedClasses['text-base']} */ ;
/** @type {__VLS_StyleScopedClasses['font-semibold']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-900']} */ ;
/** @type {__VLS_StyleScopedClasses['mt-1']} */ ;
/** @type {__VLS_StyleScopedClasses['inline-block']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-lg']} */ ;
/** @type {__VLS_StyleScopedClasses['px-2']} */ ;
/** @type {__VLS_StyleScopedClasses['py-0.5']} */ ;
/** @type {__VLS_StyleScopedClasses['text-[13px]']} */ ;
/** @type {__VLS_StyleScopedClasses['font-medium']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-400']} */ ;
/** @type {__VLS_StyleScopedClasses['transition']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:text-slate-600']} */ ;
/** @type {__VLS_StyleScopedClasses['mb-2']} */ ;
/** @type {__VLS_StyleScopedClasses['text-[13px]']} */ ;
/** @type {__VLS_StyleScopedClasses['text-indigo-600']} */ ;
/** @type {__VLS_StyleScopedClasses['text-[13px]']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-400']} */ ;
/** @type {__VLS_StyleScopedClasses['whitespace-pre-wrap']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['leading-relaxed']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-600']} */ ;
/** @type {__VLS_StyleScopedClasses['group']} */ ;
/** @type {__VLS_StyleScopedClasses['relative']} */ ;
/** @type {__VLS_StyleScopedClasses['w-1.5']} */ ;
/** @type {__VLS_StyleScopedClasses['shrink-0']} */ ;
/** @type {__VLS_StyleScopedClasses['cursor-col-resize']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-transparent']} */ ;
/** @type {__VLS_StyleScopedClasses['transition-colors']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-violet-300']} */ ;
/** @type {__VLS_StyleScopedClasses['active:bg-violet-400']} */ ;
/** @type {__VLS_StyleScopedClasses['absolute']} */ ;
/** @type {__VLS_StyleScopedClasses['inset-y-0']} */ ;
/** @type {__VLS_StyleScopedClasses['-left-1']} */ ;
/** @type {__VLS_StyleScopedClasses['-right-1']} */ ;
/** @type {__VLS_StyleScopedClasses['absolute']} */ ;
/** @type {__VLS_StyleScopedClasses['left-1/2']} */ ;
/** @type {__VLS_StyleScopedClasses['top-1/2']} */ ;
/** @type {__VLS_StyleScopedClasses['h-8']} */ ;
/** @type {__VLS_StyleScopedClasses['w-1']} */ ;
/** @type {__VLS_StyleScopedClasses['-translate-x-1/2']} */ ;
/** @type {__VLS_StyleScopedClasses['-translate-y-1/2']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-full']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-300']} */ ;
/** @type {__VLS_StyleScopedClasses['transition-colors']} */ ;
/** @type {__VLS_StyleScopedClasses['group-hover:bg-violet-400']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['shrink-0']} */ ;
/** @type {__VLS_StyleScopedClasses['flex-col']} */ ;
/** @type {__VLS_StyleScopedClasses['gap-3']} */ ;
/** @type {__VLS_StyleScopedClasses['overflow-hidden']} */ ;
/** @type {__VLS_StyleScopedClasses['border-l']} */ ;
/** @type {__VLS_StyleScopedClasses['border-gray-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-[#F8FAFC]']} */ ;
/** @type {__VLS_StyleScopedClasses['p-3']} */ ;
/** @type {__VLS_StyleScopedClasses['fixed']} */ ;
/** @type {__VLS_StyleScopedClasses['inset-0']} */ ;
/** @type {__VLS_StyleScopedClasses['z-50']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['items-center']} */ ;
/** @type {__VLS_StyleScopedClasses['justify-center']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-black/20']} */ ;
/** @type {__VLS_StyleScopedClasses['w-[320px]']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-[20px]']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-gray-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['p-6']} */ ;
/** @type {__VLS_StyleScopedClasses['text-center']} */ ;
/** @type {__VLS_StyleScopedClasses['shadow-lg']} */ ;
/** @type {__VLS_StyleScopedClasses['text-base']} */ ;
/** @type {__VLS_StyleScopedClasses['font-semibold']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-800']} */ ;
/** @type {__VLS_StyleScopedClasses['mt-2']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-500']} */ ;
/** @type {__VLS_StyleScopedClasses['mt-4']} */ ;
/** @type {__VLS_StyleScopedClasses['h-11']} */ ;
/** @type {__VLS_StyleScopedClasses['w-full']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-indigo-600']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['font-medium']} */ ;
/** @type {__VLS_StyleScopedClasses['text-white']} */ ;
/** @type {__VLS_StyleScopedClasses['transition']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-indigo-500']} */ ;
/** @type {__VLS_StyleScopedClasses['fixed']} */ ;
/** @type {__VLS_StyleScopedClasses['inset-0']} */ ;
/** @type {__VLS_StyleScopedClasses['z-50']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['items-center']} */ ;
/** @type {__VLS_StyleScopedClasses['justify-center']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-black/20']} */ ;
/** @type {__VLS_StyleScopedClasses['w-[280px]']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-[20px]']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-gray-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['p-4']} */ ;
/** @type {__VLS_StyleScopedClasses['shadow-lg']} */ ;
/** @type {__VLS_StyleScopedClasses['mb-3']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['font-semibold']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-800']} */ ;
/** @type {__VLS_StyleScopedClasses['mb-3']} */ ;
/** @type {__VLS_StyleScopedClasses['max-h-[180px]']} */ ;
/** @type {__VLS_StyleScopedClasses['space-y-1']} */ ;
/** @type {__VLS_StyleScopedClasses['overflow-y-auto']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['w-full']} */ ;
/** @type {__VLS_StyleScopedClasses['items-center']} */ ;
/** @type {__VLS_StyleScopedClasses['justify-between']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['px-3']} */ ;
/** @type {__VLS_StyleScopedClasses['py-2']} */ ;
/** @type {__VLS_StyleScopedClasses['text-left']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-700']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-violet-50']} */ ;
/** @type {__VLS_StyleScopedClasses['disabled:cursor-default']} */ ;
/** @type {__VLS_StyleScopedClasses['disabled:bg-slate-50']} */ ;
/** @type {__VLS_StyleScopedClasses['disabled:text-slate-400']} */ ;
/** @type {__VLS_StyleScopedClasses['truncate']} */ ;
/** @type {__VLS_StyleScopedClasses['text-[12px]']} */ ;
/** @type {__VLS_StyleScopedClasses['px-2']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-400']} */ ;
/** @type {__VLS_StyleScopedClasses['h-11']} */ ;
/** @type {__VLS_StyleScopedClasses['w-full']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-100']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-600']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['fixed']} */ ;
/** @type {__VLS_StyleScopedClasses['inset-0']} */ ;
/** @type {__VLS_StyleScopedClasses['z-50']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['items-center']} */ ;
/** @type {__VLS_StyleScopedClasses['justify-center']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-black/20']} */ ;
/** @type {__VLS_StyleScopedClasses['w-[280px]']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-[20px]']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-gray-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['p-4']} */ ;
/** @type {__VLS_StyleScopedClasses['shadow-lg']} */ ;
/** @type {__VLS_StyleScopedClasses['mb-3']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['font-semibold']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-800']} */ ;
/** @type {__VLS_StyleScopedClasses['mb-3']} */ ;
/** @type {__VLS_StyleScopedClasses['h-11']} */ ;
/** @type {__VLS_StyleScopedClasses['w-full']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-gray-200']} */ ;
/** @type {__VLS_StyleScopedClasses['px-3']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['outline-none']} */ ;
/** @type {__VLS_StyleScopedClasses['focus:border-indigo-500']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['gap-2']} */ ;
/** @type {__VLS_StyleScopedClasses['h-11']} */ ;
/** @type {__VLS_StyleScopedClasses['flex-1']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-100']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-600']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['h-11']} */ ;
/** @type {__VLS_StyleScopedClasses['flex-1']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-indigo-600']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['text-white']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-indigo-500']} */ ;
/** @type {__VLS_StyleScopedClasses['fixed']} */ ;
/** @type {__VLS_StyleScopedClasses['inset-0']} */ ;
/** @type {__VLS_StyleScopedClasses['z-50']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['items-center']} */ ;
/** @type {__VLS_StyleScopedClasses['justify-center']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-950/30']} */ ;
/** @type {__VLS_StyleScopedClasses['backdrop-blur-[2px]']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-[20px]']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-white/10']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-900/90']} */ ;
/** @type {__VLS_StyleScopedClasses['px-8']} */ ;
/** @type {__VLS_StyleScopedClasses['py-6']} */ ;
/** @type {__VLS_StyleScopedClasses['text-center']} */ ;
/** @type {__VLS_StyleScopedClasses['text-white']} */ ;
/** @type {__VLS_StyleScopedClasses['shadow-lg']} */ ;
/** @type {__VLS_StyleScopedClasses['mx-auto']} */ ;
/** @type {__VLS_StyleScopedClasses['h-10']} */ ;
/** @type {__VLS_StyleScopedClasses['w-10']} */ ;
/** @type {__VLS_StyleScopedClasses['animate-spin']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-full']} */ ;
/** @type {__VLS_StyleScopedClasses['border-2']} */ ;
/** @type {__VLS_StyleScopedClasses['border-white/20']} */ ;
/** @type {__VLS_StyleScopedClasses['border-t-white']} */ ;
/** @type {__VLS_StyleScopedClasses['mt-3']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
var __VLS_dollars;
const __VLS_self = (await import('vue')).defineComponent({
    setup() {
        return {
            FileSidebar: FileSidebar,
            ImportPanel: ImportPanel,
            LearningNavPanel: LearningNavPanel,
            AiChatPanel: AiChatPanel,
            renderMarkdown: renderMarkdown,
            markdown: markdown,
            concept: concept,
            userId: userId,
            loggedInUserId: loggedInUserId,
            maxDepth: maxDepth,
            isLoading: isLoading,
            isNavigating: isNavigating,
            statusText: statusText,
            importedFileName: importedFileName,
            graphRoot: graphRoot,
            selectedFileId: selectedFileId,
            selectedFileGroupId: selectedFileGroupId,
            files: files,
            fileGroups: fileGroups,
            newGroupName: newGroupName,
            menuOpen: menuOpen,
            renameTarget: renameTarget,
            renameValue: renameValue,
            addFileToGroupTarget: addFileToGroupTarget,
            showLoginPrompt: showLoginPrompt,
            isSwitching: isSwitching,
            isRefreshing: isRefreshing,
            selectedNodeDetail: selectedNodeDetail,
            isExplaining: isExplaining,
            chatMessages: chatMessages,
            chatInput: chatInput,
            isChatting: isChatting,
            currentConversationId: currentConversationId,
            isLightRAGMode: isLightRAGMode,
            graphSearch: graphSearch,
            graphStatusFilter: graphStatusFilter,
            graphLayoutMode: graphLayoutMode,
            isFocusMode: isFocusMode,
            leftWidth: leftWidth,
            rightWidth: rightWidth,
            onDividerMousedown: onDividerMousedown,
            canGenerate: canGenerate,
            canNavigate: canNavigate,
            handleLogin: handleLogin,
            handleRefresh: handleRefresh,
            handleCreateGroup: handleCreateGroup,
            handleDeleteFile: handleDeleteFile,
            handleDeleteGroup: handleDeleteGroup,
            openRenameDialog: openRenameDialog,
            handleRename: handleRename,
            handleTogglePin: handleTogglePin,
            handleAddFileToGroup: handleAddFileToGroup,
            selectFile: selectFile,
            selectFileGroup: selectFileGroup,
            handleClearConversation: handleClearConversation,
            sendChatMessage: sendChatMessage,
            handleChatImageUpload: handleChatImageUpload,
            fetchAllGraph: fetchAllGraph,
            handleUpload: handleUpload,
            handleImportMarkdownFile: handleImportMarkdownFile,
            handleLeftUploadFile: handleLeftUploadFile,
            handleLeftUploadFileGroup: handleLeftUploadFileGroup,
            handlePathNavigate: handlePathNavigate,
            resetFocus: resetFocus,
            searchGraph: searchGraph,
            applyStatusFilter: applyStatusFilter,
            applyLayoutMode: applyLayoutMode,
            fitGraph: fitGraph,
            zoomGraph: zoomGraph,
        };
    },
});
export default (await import('vue')).defineComponent({
    setup() {
        return {};
    },
});
; /* PartiallyEnd: #4569/main.vue */
