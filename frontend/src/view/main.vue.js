import { Graph } from "@antv/g6";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { marked } from "marked";
import { chatWithContext, deleteConversation, explainConcept, getConversation, getGraphAll, getGraphPath, getLearningPath, getNodeNeighbors, saveMessage, uploadNoteLangChain, } from "../api/graph";
import { buildFocusSet, buildStyledGraph, getEdgeConfig, getLayoutConfig, getNodeConfig, preprocessGraphData, } from "../graph/g6-config";
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
const selectedNodeDetail = ref(null);
const isExplaining = ref(false);
const chatMessages = ref([]);
const chatInput = ref("");
const isChatting = ref(false);
const currentConversationId = ref("");
const isLightRAGMode = ref(true);
const expandedNodes = ref(new Set());
let graph = null;
let graphRawData = { nodes: [], edges: [] };
let resizeObserver = null;
const canGenerate = computed(() => markdown.value.trim().length > 0);
const canNavigate = computed(() => concept.value.trim().length > 0);
function currentUserId() {
    return loggedInUserId.value || undefined;
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
    return h ? String(h) : "";
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
    statusText.value = `已登录：${u}`;
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
        else {
            void showNodeDetail(id);
        }
    });
    graph.on("canvas:click", () => {
        selectedNodeDetail.value = null;
    });
}
async function renderGraph(data, focusMode = false, pathData = [], layoutType = "force") {
    if (!graph)
        return;
    const focusSet = focusMode ? buildFocusSet(pathData) : undefined;
    const styled = buildStyledGraph(data, focusSet);
    graph.setLayout(styled.nodes.some((n) => n.x !== undefined)
        ? { type: "preset", padding: 50 }
        : getLayoutConfig(layoutType));
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
    const raw = await getGraphAll({
        file_id: selectedFileId.value || undefined,
        file_group_id: selectedFileGroupId.value || undefined,
        user_id: currentUserId(),
    });
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
async function handlePathNavigate() {
    if (!canNavigate.value)
        return;
    isNavigating.value = true;
    statusText.value = "计算逆向学习路径…";
    try {
        const r = await getGraphPath(concept.value.trim(), currentUserId(), maxDepth.value);
        const hasRelated = r.all_related && r.all_related.nodes && r.all_related.nodes.length > 0;
        await renderGraph(hasRelated ? r.all_related : graphRawData, true, hasRelated ? [r.all_related] : r.paths, "force");
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
    await renderGraph(graphRawData, false, [], "force");
    isNavigating.value = false;
    statusText.value = "已退出专注模式";
}
onMounted(async () => {
    await initGraph();
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
__VLS_asFunctionalElement(__VLS_intrinsicElements.section, __VLS_intrinsicElements.section)({
    ...{ class: "flex min-w-0 flex-1 flex-col overflow-hidden" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.header, __VLS_intrinsicElements.header)({
    ...{ class: "flex items-center gap-3 border-b border-gray-200 bg-white px-5 py-3" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({});
__VLS_asFunctionalElement(__VLS_intrinsicElements.h1, __VLS_intrinsicElements.h1)({
    ...{ class: "text-lg font-semibold text-slate-900" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
    ...{ class: "text-[13px] text-slate-500" },
});
(__VLS_ctx.statusText);
__VLS_asFunctionalElement(__VLS_intrinsicElements.div)({
    ...{ class: "flex-1" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
    ...{ onClick: (...[$event]) => {
            __VLS_ctx.isLightRAGMode = !__VLS_ctx.isLightRAGMode;
            __VLS_ctx.fetchAllGraph();
        } },
    type: "button",
    ...{ class: "h-11 rounded-xl border border-gray-200 bg-white px-4 text-sm font-medium text-slate-700 transition duration-200 hover:bg-gray-100" },
});
(__VLS_ctx.isLightRAGMode ? "渐进式展开" : "显示全图");
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
    ...{ class: "relative m-4 min-h-0 flex-1 overflow-hidden rounded-[18px] border border-gray-200 bg-slate-950 shadow-sm" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div)({
    ref: "graphRoot",
    ...{ class: "absolute inset-0" },
});
/** @type {typeof __VLS_ctx.graphRoot} */ ;
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
__VLS_asFunctionalElement(__VLS_intrinsicElements.aside, __VLS_intrinsicElements.aside)({
    ...{ class: "flex w-[380px] shrink-0 flex-col gap-4 overflow-hidden border-l border-gray-200 bg-[#F8FAFC] p-4" },
});
/** @type {[typeof ImportPanel, ]} */ ;
// @ts-ignore
const __VLS_0 = __VLS_asFunctionalComponent(ImportPanel, new ImportPanel({
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
const __VLS_1 = __VLS_0({
    ...{ 'onLogin': {} },
    ...{ 'onImportFile': {} },
    ...{ 'onGenerate': {} },
    markdown: (__VLS_ctx.markdown),
    userId: (__VLS_ctx.userId),
    importedFileName: (__VLS_ctx.importedFileName),
    loggedInUserId: (__VLS_ctx.loggedInUserId),
    isLoading: (__VLS_ctx.isLoading),
    canGenerate: (__VLS_ctx.canGenerate),
}, ...__VLS_functionalComponentArgsRest(__VLS_0));
let __VLS_3;
let __VLS_4;
let __VLS_5;
const __VLS_6 = {
    onLogin: (__VLS_ctx.handleLogin)
};
const __VLS_7 = {
    onImportFile: (__VLS_ctx.handleImportMarkdownFile)
};
const __VLS_8 = {
    onGenerate: (__VLS_ctx.handleUpload)
};
var __VLS_2;
/** @type {[typeof LearningNavPanel, ]} */ ;
// @ts-ignore
const __VLS_9 = __VLS_asFunctionalComponent(LearningNavPanel, new LearningNavPanel({
    ...{ 'onNavigate': {} },
    ...{ 'onReset': {} },
    concept: (__VLS_ctx.concept),
    maxDepth: (__VLS_ctx.maxDepth),
    canNavigate: (__VLS_ctx.canNavigate),
    isLoading: (__VLS_ctx.isLoading),
    isNavigating: (__VLS_ctx.isNavigating),
}));
const __VLS_10 = __VLS_9({
    ...{ 'onNavigate': {} },
    ...{ 'onReset': {} },
    concept: (__VLS_ctx.concept),
    maxDepth: (__VLS_ctx.maxDepth),
    canNavigate: (__VLS_ctx.canNavigate),
    isLoading: (__VLS_ctx.isLoading),
    isNavigating: (__VLS_ctx.isNavigating),
}, ...__VLS_functionalComponentArgsRest(__VLS_9));
let __VLS_12;
let __VLS_13;
let __VLS_14;
const __VLS_15 = {
    onNavigate: (__VLS_ctx.handlePathNavigate)
};
const __VLS_16 = {
    onReset: (__VLS_ctx.resetFocus)
};
var __VLS_11;
/** @type {[typeof AiChatPanel, ]} */ ;
// @ts-ignore
const __VLS_17 = __VLS_asFunctionalComponent(AiChatPanel, new AiChatPanel({
    ...{ 'onSend': {} },
    ...{ 'onClear': {} },
    ...{ 'onUploadImage': {} },
    chatInput: (__VLS_ctx.chatInput),
    messages: (__VLS_ctx.chatMessages),
    isChatting: (__VLS_ctx.isChatting),
    hasConversation: (!!__VLS_ctx.currentConversationId),
    renderMarkdown: (__VLS_ctx.renderMarkdown),
}));
const __VLS_18 = __VLS_17({
    ...{ 'onSend': {} },
    ...{ 'onClear': {} },
    ...{ 'onUploadImage': {} },
    chatInput: (__VLS_ctx.chatInput),
    messages: (__VLS_ctx.chatMessages),
    isChatting: (__VLS_ctx.isChatting),
    hasConversation: (!!__VLS_ctx.currentConversationId),
    renderMarkdown: (__VLS_ctx.renderMarkdown),
}, ...__VLS_functionalComponentArgsRest(__VLS_17));
let __VLS_20;
let __VLS_21;
let __VLS_22;
const __VLS_23 = {
    onSend: (...[$event]) => {
        __VLS_ctx.sendChatMessage();
    }
};
const __VLS_24 = {
    onClear: (__VLS_ctx.handleClearConversation)
};
const __VLS_25 = {
    onUploadImage: (__VLS_ctx.handleChatImageUpload)
};
var __VLS_19;
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
/** @type {__VLS_StyleScopedClasses['text-lg']} */ ;
/** @type {__VLS_StyleScopedClasses['font-semibold']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-900']} */ ;
/** @type {__VLS_StyleScopedClasses['text-[13px]']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-500']} */ ;
/** @type {__VLS_StyleScopedClasses['flex-1']} */ ;
/** @type {__VLS_StyleScopedClasses['h-11']} */ ;
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
/** @type {__VLS_StyleScopedClasses['m-4']} */ ;
/** @type {__VLS_StyleScopedClasses['min-h-0']} */ ;
/** @type {__VLS_StyleScopedClasses['flex-1']} */ ;
/** @type {__VLS_StyleScopedClasses['overflow-hidden']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-[18px]']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-gray-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-950']} */ ;
/** @type {__VLS_StyleScopedClasses['shadow-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['absolute']} */ ;
/** @type {__VLS_StyleScopedClasses['inset-0']} */ ;
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
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['w-[380px]']} */ ;
/** @type {__VLS_StyleScopedClasses['shrink-0']} */ ;
/** @type {__VLS_StyleScopedClasses['flex-col']} */ ;
/** @type {__VLS_StyleScopedClasses['gap-4']} */ ;
/** @type {__VLS_StyleScopedClasses['overflow-hidden']} */ ;
/** @type {__VLS_StyleScopedClasses['border-l']} */ ;
/** @type {__VLS_StyleScopedClasses['border-gray-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-[#F8FAFC]']} */ ;
/** @type {__VLS_StyleScopedClasses['p-4']} */ ;
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
            selectedNodeDetail: selectedNodeDetail,
            isExplaining: isExplaining,
            chatMessages: chatMessages,
            chatInput: chatInput,
            isChatting: isChatting,
            currentConversationId: currentConversationId,
            isLightRAGMode: isLightRAGMode,
            canGenerate: canGenerate,
            canNavigate: canNavigate,
            handleLogin: handleLogin,
            handleClearConversation: handleClearConversation,
            sendChatMessage: sendChatMessage,
            handleChatImageUpload: handleChatImageUpload,
            fetchAllGraph: fetchAllGraph,
            handleUpload: handleUpload,
            handleImportMarkdownFile: handleImportMarkdownFile,
            handlePathNavigate: handlePathNavigate,
            resetFocus: resetFocus,
        };
    },
});
export default (await import('vue')).defineComponent({
    setup() {
        return {};
    },
});
; /* PartiallyEnd: #4569/main.vue */
