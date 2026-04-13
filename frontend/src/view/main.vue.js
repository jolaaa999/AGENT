import { Graph } from "@antv/g6";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { explainConcept, getGraphAll, getGraphPath, uploadNote } from "../api/graph";
import { buildFocusSet, buildStyledGraph } from "../graph/g6-config";
const markdown = ref("");
const concept = ref("");
const userId = ref("");
const maxDepth = ref(3);
const isLoading = ref(false);
const isNavigating = ref(false);
const statusText = ref("图谱待生成");
const importedFileName = ref("");
const graphRoot = ref(null);
const selectedNodeDetail = ref(null);
const isExplaining = ref(false);
let graph = null;
let graphRawData = { nodes: [], edges: [] };
let resizeObserver = null;
const canGenerate = computed(() => markdown.value.trim().length > 0);
const canNavigate = computed(() => concept.value.trim().length > 0);
function tooltipHtml(reason) {
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
function extractMarkdownSnippets(concept, content) {
    if (!concept.trim() || !content.trim())
        return [];
    const lines = content.split(/\r?\n/);
    const normalizedConcept = concept.toLowerCase();
    const snippets = [];
    const usedRanges = new Set();
    for (let i = 0; i < lines.length; i += 1) {
        const line = lines[i];
        if (!line.toLowerCase().includes(normalizedConcept))
            continue;
        const start = Math.max(0, i - 1);
        const end = Math.min(lines.length - 1, i + 1);
        const rangeKey = `${start}-${end}`;
        if (usedRanges.has(rangeKey))
            continue;
        usedRanges.add(rangeKey);
        const snippet = lines
            .slice(start, end + 1)
            .join("\n")
            .trim();
        if (snippet)
            snippets.push(snippet);
        if (snippets.length >= 3)
            break;
    }
    return snippets;
}
function buildAIExplanation(node, snippets) {
    const statusText = node.status === "correct"
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
function resolveNodeIdFromEvent(event) {
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
async function showNodeDetail(nodeId) {
    const node = graphRawData.nodes.find((item) => item.id === nodeId);
    if (!node)
        return;
    const snippets = extractMarkdownSnippets(node.label || node.id, markdown.value);
    selectedNodeDetail.value = {
        id: node.id,
        label: node.label || node.id,
        type: node.type || String(node.data?.type ?? ""),
        status: node.status || String(node.data?.status ?? ""),
        reason: node.reason || String(node.data?.reason ?? ""),
        snippets,
        aiExplanation: buildAIExplanation({
            label: node.label || node.id,
            status: node.status || String(node.data?.status ?? ""),
            reason: node.reason || String(node.data?.reason ?? "")
        }, snippets)
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
    }
    catch (error) {
        if (selectedNodeDetail.value?.id === node.id) {
            selectedNodeDetail.value.aiExplanation = `讲解生成失败：${error.message}`;
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
                enable: (event) => {
                    if (event.targetType !== "node")
                        return false;
                    const status = String(event.target?.data?.status ?? "");
                    return status === "error" || status === "supplement";
                },
                getContent: (event) => tooltipHtml(String(event.target?.data?.reason ?? ""))
            }
        ]
    });
    await graph.render();
    graph.on("node:click", (event) => {
        const nodeId = resolveNodeIdFromEvent(event);
        if (!nodeId)
            return;
        void showNodeDetail(nodeId);
    });
    graph.on("element:click", (event) => {
        if (event?.targetType !== "node")
            return;
        const nodeId = resolveNodeIdFromEvent(event);
        if (!nodeId)
            return;
        void showNodeDetail(nodeId);
    });
    graph.on("canvas:click", () => {
        selectedNodeDetail.value = null;
    });
}
async function renderGraph(data, focusMode = false, pathData = []) {
    if (!graph)
        return;
    const focusSet = focusMode ? buildFocusSet(pathData) : undefined;
    const styled = buildStyledGraph(data, focusSet);
    graph.setData(styled);
    await graph.render();
}
async function fetchAllGraph() {
    graphRawData = await getGraphAll(userId.value.trim() || undefined);
    await renderGraph(graphRawData);
    statusText.value = `图谱已加载：${graphRawData.nodes.length} 节点 / ${graphRawData.edges.length} 连线`;
}
async function handleUpload() {
    if (!canGenerate.value)
        return;
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
    }
    catch (error) {
        statusText.value = `生成失败：${error.message}`;
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
    }
    catch (error) {
        statusText.value = `读取文件失败：${error.message}`;
    }
    finally {
        input.value = "";
    }
}
async function handlePathNavigate() {
    if (!canNavigate.value)
        return;
    statusText.value = "正在计算逆向学习路径...";
    try {
        const result = await getGraphPath(concept.value.trim(), userId.value.trim() || undefined, maxDepth.value);
        await renderGraph(graphRawData, true, result.paths);
        isNavigating.value = true;
        statusText.value = `专注模式已开启：命中 ${result.paths.length} 条依赖路径`;
    }
    catch (error) {
        statusText.value = `路径查询失败：${error.message}`;
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
            if (!entry || !graph)
                return;
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
debugger; /* PartiallyEnd: #3632/scriptSetup.vue */
const __VLS_ctx = {};
let __VLS_components;
let __VLS_directives;
__VLS_asFunctionalElement(__VLS_intrinsicElements.main, __VLS_intrinsicElements.main)({
    ...{ class: "relative min-h-screen bg-slate-100/80 p-5 md:p-8" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
    ...{ class: "grid w-full grid-cols-1 gap-5 xl:grid-cols-[1fr_2fr]" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.section, __VLS_intrinsicElements.section)({
    ...{ class: "rounded-2xl border border-slate-200/80 bg-white/95 p-6 shadow-[0_10px_40px_rgba(15,23,42,0.05)] xl:h-[calc(100vh-96px)] xl:overflow-y-auto" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.header, __VLS_intrinsicElements.header)({
    ...{ class: "mb-5" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.h1, __VLS_intrinsicElements.h1)({
    ...{ class: "text-[22px] font-semibold tracking-tight text-slate-900" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
    ...{ class: "mt-2 text-sm leading-relaxed text-slate-500" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.label, __VLS_intrinsicElements.label)({
    ...{ class: "mb-2 block text-xs font-medium uppercase tracking-[0.12em] text-slate-400" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
    ...{ class: "mb-3 flex items-center justify-between gap-3 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2.5" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.label, __VLS_intrinsicElements.label)({
    ...{ class: "inline-flex cursor-pointer items-center rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-xs font-medium text-slate-700 transition hover:bg-slate-50" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.input)({
    ...{ onChange: (__VLS_ctx.handleImportMarkdownFile) },
    ...{ class: "hidden" },
    type: "file",
    accept: ".md,text/markdown",
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.span, __VLS_intrinsicElements.span)({
    ...{ class: "truncate text-xs text-slate-500" },
});
(__VLS_ctx.importedFileName || "未选择文件");
__VLS_asFunctionalElement(__VLS_intrinsicElements.textarea)({
    value: (__VLS_ctx.markdown),
    ...{ class: "h-72 w-full resize-none rounded-xl border border-slate-200 bg-slate-50/70 p-4 text-sm leading-relaxed text-slate-700 outline-none transition focus:border-blue-400 focus:bg-white xl:h-[min(44vh,420px)]" },
    placeholder: "在这里粘贴你的课程笔记、读书摘要或错题复盘...",
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
    ...{ class: "mt-4 grid grid-cols-1 gap-3 sm:grid-cols-[1fr_auto]" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.input)({
    ...{ class: "rounded-xl border border-slate-200 bg-white px-3 py-2.5 text-sm text-slate-700 outline-none focus:border-slate-400" },
    placeholder: "可选：user_id（用于多用户隔离）",
});
(__VLS_ctx.userId);
__VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
    ...{ onClick: (__VLS_ctx.handleUpload) },
    disabled: (!__VLS_ctx.canGenerate || __VLS_ctx.isLoading),
    ...{ class: "rounded-xl bg-slate-900 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-50" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
    ...{ class: "mt-6 border-t border-slate-200 pt-5" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.label, __VLS_intrinsicElements.label)({
    ...{ class: "mb-2 block text-xs font-medium uppercase tracking-[0.12em] text-slate-400" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
    ...{ class: "grid grid-cols-1 gap-3 sm:grid-cols-[1fr_110px]" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.input)({
    ...{ class: "rounded-xl border border-slate-200 bg-white px-3 py-2.5 text-sm text-slate-700 outline-none focus:border-violet-400" },
    placeholder: "输入目标概念，如：机器学习",
});
(__VLS_ctx.concept);
__VLS_asFunctionalElement(__VLS_intrinsicElements.input)({
    type: "number",
    min: "1",
    max: "6",
    ...{ class: "rounded-xl border border-slate-200 bg-white px-3 py-2.5 text-sm text-slate-700 outline-none focus:border-violet-400" },
    placeholder: "深度",
});
(__VLS_ctx.maxDepth);
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
    ...{ class: "mt-3 flex gap-3" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
    ...{ onClick: (__VLS_ctx.handlePathNavigate) },
    disabled: (!__VLS_ctx.canNavigate || __VLS_ctx.isLoading),
    ...{ class: "rounded-xl bg-violet-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-violet-500 disabled:cursor-not-allowed disabled:opacity-50" },
});
if (__VLS_ctx.isNavigating) {
    __VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
        ...{ onClick: (__VLS_ctx.resetFocus) },
        ...{ class: "rounded-xl border border-slate-300 bg-white px-4 py-2.5 text-sm font-medium text-slate-600 transition hover:bg-slate-50" },
    });
}
__VLS_asFunctionalElement(__VLS_intrinsicElements.section, __VLS_intrinsicElements.section)({
    ...{ class: "relative overflow-hidden rounded-2xl border border-slate-200/80 bg-white shadow-[0_10px_40px_rgba(15,23,42,0.05)] xl:h-[calc(100vh-96px)]" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
    ...{ class: "flex items-center justify-between border-b border-slate-200 px-5 py-3.5" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
    ...{ class: "text-sm font-medium text-slate-700" },
});
__VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
    ...{ class: "text-xs text-slate-500" },
});
(__VLS_ctx.statusText);
__VLS_asFunctionalElement(__VLS_intrinsicElements.div)({
    ref: "graphRoot",
    ...{ class: "h-[65vh] min-h-[460px] w-full bg-[radial-gradient(circle_at_20%_20%,#f8fafc,transparent_45%),radial-gradient(circle_at_80%_90%,#eef2ff,transparent_40%)] xl:h-[calc(100%-52px)]" },
});
/** @type {typeof __VLS_ctx.graphRoot} */ ;
if (__VLS_ctx.selectedNodeDetail) {
    __VLS_asFunctionalElement(__VLS_intrinsicElements.aside, __VLS_intrinsicElements.aside)({
        ...{ class: "absolute right-4 top-16 z-20 w-[340px] max-w-[calc(100%-2rem)] rounded-xl border border-slate-200 bg-white/95 p-4 shadow-xl backdrop-blur" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ class: "mb-2 flex items-start justify-between gap-3" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.h3, __VLS_intrinsicElements.h3)({
        ...{ class: "text-base font-semibold text-slate-900" },
    });
    (__VLS_ctx.selectedNodeDetail.label);
    __VLS_asFunctionalElement(__VLS_intrinsicElements.button, __VLS_intrinsicElements.button)({
        ...{ onClick: (...[$event]) => {
                if (!(__VLS_ctx.selectedNodeDetail))
                    return;
                __VLS_ctx.selectedNodeDetail = null;
            } },
        ...{ class: "rounded-md px-2 py-1 text-xs text-slate-500 transition hover:bg-slate-100 hover:text-slate-700" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ class: "mb-3 flex flex-wrap gap-2 text-xs" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.span, __VLS_intrinsicElements.span)({
        ...{ class: "rounded-md bg-slate-100 px-2 py-1 text-slate-600" },
    });
    (__VLS_ctx.selectedNodeDetail.type || "Concept");
    __VLS_asFunctionalElement(__VLS_intrinsicElements.span, __VLS_intrinsicElements.span)({
        ...{ class: "rounded-md bg-slate-100 px-2 py-1 text-slate-600" },
    });
    (__VLS_ctx.selectedNodeDetail.status || "unknown");
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ class: "space-y-2" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
        ...{ class: "text-xs font-medium text-slate-500" },
    });
    if (__VLS_ctx.selectedNodeDetail.snippets.length) {
        for (const [snippet, index] of __VLS_getVForSourceType((__VLS_ctx.selectedNodeDetail.snippets))) {
            __VLS_asFunctionalElement(__VLS_intrinsicElements.pre, __VLS_intrinsicElements.pre)({
                key: (`${__VLS_ctx.selectedNodeDetail.id}-${index}`),
                ...{ class: "whitespace-pre-wrap rounded-lg border border-slate-200 bg-slate-50 p-2.5 text-xs leading-relaxed text-slate-700" },
            });
            (snippet);
        }
    }
    else {
        __VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
            ...{ class: "rounded-lg border border-dashed border-slate-200 bg-slate-50 p-2.5 text-xs text-slate-500" },
        });
    }
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ class: "mt-3 rounded-lg border border-violet-200 bg-violet-50/60 p-2.5" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
        ...{ class: "text-xs font-medium text-violet-700" },
    });
    if (__VLS_ctx.isExplaining) {
        __VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
            ...{ class: "mt-1 text-xs text-violet-700" },
        });
    }
    __VLS_asFunctionalElement(__VLS_intrinsicElements.pre, __VLS_intrinsicElements.pre)({
        ...{ class: "mt-1 whitespace-pre-wrap text-xs leading-relaxed text-slate-700" },
    });
    (__VLS_ctx.selectedNodeDetail.aiExplanation);
}
if (__VLS_ctx.isLoading) {
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ class: "fixed inset-0 z-50 flex items-center justify-center bg-slate-950/30 backdrop-blur-[2px]" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div, __VLS_intrinsicElements.div)({
        ...{ class: "rounded-2xl border border-white/20 bg-slate-900/90 px-8 py-6 text-center text-white shadow-2xl" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.div)({
        ...{ class: "mx-auto h-10 w-10 animate-spin rounded-full border-2 border-white/20 border-t-white" },
    });
    __VLS_asFunctionalElement(__VLS_intrinsicElements.p, __VLS_intrinsicElements.p)({
        ...{ class: "mt-3 text-sm tracking-wide" },
    });
}
/** @type {__VLS_StyleScopedClasses['relative']} */ ;
/** @type {__VLS_StyleScopedClasses['min-h-screen']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-100/80']} */ ;
/** @type {__VLS_StyleScopedClasses['p-5']} */ ;
/** @type {__VLS_StyleScopedClasses['md:p-8']} */ ;
/** @type {__VLS_StyleScopedClasses['grid']} */ ;
/** @type {__VLS_StyleScopedClasses['w-full']} */ ;
/** @type {__VLS_StyleScopedClasses['grid-cols-1']} */ ;
/** @type {__VLS_StyleScopedClasses['gap-5']} */ ;
/** @type {__VLS_StyleScopedClasses['xl:grid-cols-[1fr_2fr]']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-2xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200/80']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white/95']} */ ;
/** @type {__VLS_StyleScopedClasses['p-6']} */ ;
/** @type {__VLS_StyleScopedClasses['shadow-[0_10px_40px_rgba(15,23,42,0.05)]']} */ ;
/** @type {__VLS_StyleScopedClasses['xl:h-[calc(100vh-96px)]']} */ ;
/** @type {__VLS_StyleScopedClasses['xl:overflow-y-auto']} */ ;
/** @type {__VLS_StyleScopedClasses['mb-5']} */ ;
/** @type {__VLS_StyleScopedClasses['text-[22px]']} */ ;
/** @type {__VLS_StyleScopedClasses['font-semibold']} */ ;
/** @type {__VLS_StyleScopedClasses['tracking-tight']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-900']} */ ;
/** @type {__VLS_StyleScopedClasses['mt-2']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['leading-relaxed']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-500']} */ ;
/** @type {__VLS_StyleScopedClasses['mb-2']} */ ;
/** @type {__VLS_StyleScopedClasses['block']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['font-medium']} */ ;
/** @type {__VLS_StyleScopedClasses['uppercase']} */ ;
/** @type {__VLS_StyleScopedClasses['tracking-[0.12em]']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-400']} */ ;
/** @type {__VLS_StyleScopedClasses['mb-3']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['items-center']} */ ;
/** @type {__VLS_StyleScopedClasses['justify-between']} */ ;
/** @type {__VLS_StyleScopedClasses['gap-3']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-50']} */ ;
/** @type {__VLS_StyleScopedClasses['px-3']} */ ;
/** @type {__VLS_StyleScopedClasses['py-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['inline-flex']} */ ;
/** @type {__VLS_StyleScopedClasses['cursor-pointer']} */ ;
/** @type {__VLS_StyleScopedClasses['items-center']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-lg']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-300']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['px-3']} */ ;
/** @type {__VLS_StyleScopedClasses['py-1.5']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['font-medium']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-700']} */ ;
/** @type {__VLS_StyleScopedClasses['transition']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-slate-50']} */ ;
/** @type {__VLS_StyleScopedClasses['hidden']} */ ;
/** @type {__VLS_StyleScopedClasses['truncate']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-500']} */ ;
/** @type {__VLS_StyleScopedClasses['h-72']} */ ;
/** @type {__VLS_StyleScopedClasses['w-full']} */ ;
/** @type {__VLS_StyleScopedClasses['resize-none']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-50/70']} */ ;
/** @type {__VLS_StyleScopedClasses['p-4']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['leading-relaxed']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-700']} */ ;
/** @type {__VLS_StyleScopedClasses['outline-none']} */ ;
/** @type {__VLS_StyleScopedClasses['transition']} */ ;
/** @type {__VLS_StyleScopedClasses['focus:border-blue-400']} */ ;
/** @type {__VLS_StyleScopedClasses['focus:bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['xl:h-[min(44vh,420px)]']} */ ;
/** @type {__VLS_StyleScopedClasses['mt-4']} */ ;
/** @type {__VLS_StyleScopedClasses['grid']} */ ;
/** @type {__VLS_StyleScopedClasses['grid-cols-1']} */ ;
/** @type {__VLS_StyleScopedClasses['gap-3']} */ ;
/** @type {__VLS_StyleScopedClasses['sm:grid-cols-[1fr_auto]']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['px-3']} */ ;
/** @type {__VLS_StyleScopedClasses['py-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-700']} */ ;
/** @type {__VLS_StyleScopedClasses['outline-none']} */ ;
/** @type {__VLS_StyleScopedClasses['focus:border-slate-400']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-900']} */ ;
/** @type {__VLS_StyleScopedClasses['px-4']} */ ;
/** @type {__VLS_StyleScopedClasses['py-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['font-medium']} */ ;
/** @type {__VLS_StyleScopedClasses['text-white']} */ ;
/** @type {__VLS_StyleScopedClasses['transition']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-slate-800']} */ ;
/** @type {__VLS_StyleScopedClasses['disabled:cursor-not-allowed']} */ ;
/** @type {__VLS_StyleScopedClasses['disabled:opacity-50']} */ ;
/** @type {__VLS_StyleScopedClasses['mt-6']} */ ;
/** @type {__VLS_StyleScopedClasses['border-t']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['pt-5']} */ ;
/** @type {__VLS_StyleScopedClasses['mb-2']} */ ;
/** @type {__VLS_StyleScopedClasses['block']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['font-medium']} */ ;
/** @type {__VLS_StyleScopedClasses['uppercase']} */ ;
/** @type {__VLS_StyleScopedClasses['tracking-[0.12em]']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-400']} */ ;
/** @type {__VLS_StyleScopedClasses['grid']} */ ;
/** @type {__VLS_StyleScopedClasses['grid-cols-1']} */ ;
/** @type {__VLS_StyleScopedClasses['gap-3']} */ ;
/** @type {__VLS_StyleScopedClasses['sm:grid-cols-[1fr_110px]']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['px-3']} */ ;
/** @type {__VLS_StyleScopedClasses['py-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-700']} */ ;
/** @type {__VLS_StyleScopedClasses['outline-none']} */ ;
/** @type {__VLS_StyleScopedClasses['focus:border-violet-400']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['px-3']} */ ;
/** @type {__VLS_StyleScopedClasses['py-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-700']} */ ;
/** @type {__VLS_StyleScopedClasses['outline-none']} */ ;
/** @type {__VLS_StyleScopedClasses['focus:border-violet-400']} */ ;
/** @type {__VLS_StyleScopedClasses['mt-3']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['gap-3']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-violet-600']} */ ;
/** @type {__VLS_StyleScopedClasses['px-4']} */ ;
/** @type {__VLS_StyleScopedClasses['py-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['font-medium']} */ ;
/** @type {__VLS_StyleScopedClasses['text-white']} */ ;
/** @type {__VLS_StyleScopedClasses['transition']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-violet-500']} */ ;
/** @type {__VLS_StyleScopedClasses['disabled:cursor-not-allowed']} */ ;
/** @type {__VLS_StyleScopedClasses['disabled:opacity-50']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-300']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['px-4']} */ ;
/** @type {__VLS_StyleScopedClasses['py-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['font-medium']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-600']} */ ;
/** @type {__VLS_StyleScopedClasses['transition']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-slate-50']} */ ;
/** @type {__VLS_StyleScopedClasses['relative']} */ ;
/** @type {__VLS_StyleScopedClasses['overflow-hidden']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-2xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200/80']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white']} */ ;
/** @type {__VLS_StyleScopedClasses['shadow-[0_10px_40px_rgba(15,23,42,0.05)]']} */ ;
/** @type {__VLS_StyleScopedClasses['xl:h-[calc(100vh-96px)]']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['items-center']} */ ;
/** @type {__VLS_StyleScopedClasses['justify-between']} */ ;
/** @type {__VLS_StyleScopedClasses['border-b']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['px-5']} */ ;
/** @type {__VLS_StyleScopedClasses['py-3.5']} */ ;
/** @type {__VLS_StyleScopedClasses['text-sm']} */ ;
/** @type {__VLS_StyleScopedClasses['font-medium']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-700']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-500']} */ ;
/** @type {__VLS_StyleScopedClasses['h-[65vh]']} */ ;
/** @type {__VLS_StyleScopedClasses['min-h-[460px]']} */ ;
/** @type {__VLS_StyleScopedClasses['w-full']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-[radial-gradient(circle_at_20%_20%,#f8fafc,transparent_45%),radial-gradient(circle_at_80%_90%,#eef2ff,transparent_40%)]']} */ ;
/** @type {__VLS_StyleScopedClasses['xl:h-[calc(100%-52px)]']} */ ;
/** @type {__VLS_StyleScopedClasses['absolute']} */ ;
/** @type {__VLS_StyleScopedClasses['right-4']} */ ;
/** @type {__VLS_StyleScopedClasses['top-16']} */ ;
/** @type {__VLS_StyleScopedClasses['z-20']} */ ;
/** @type {__VLS_StyleScopedClasses['w-[340px]']} */ ;
/** @type {__VLS_StyleScopedClasses['max-w-[calc(100%-2rem)]']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-white/95']} */ ;
/** @type {__VLS_StyleScopedClasses['p-4']} */ ;
/** @type {__VLS_StyleScopedClasses['shadow-xl']} */ ;
/** @type {__VLS_StyleScopedClasses['backdrop-blur']} */ ;
/** @type {__VLS_StyleScopedClasses['mb-2']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['items-start']} */ ;
/** @type {__VLS_StyleScopedClasses['justify-between']} */ ;
/** @type {__VLS_StyleScopedClasses['gap-3']} */ ;
/** @type {__VLS_StyleScopedClasses['text-base']} */ ;
/** @type {__VLS_StyleScopedClasses['font-semibold']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-900']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-md']} */ ;
/** @type {__VLS_StyleScopedClasses['px-2']} */ ;
/** @type {__VLS_StyleScopedClasses['py-1']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-500']} */ ;
/** @type {__VLS_StyleScopedClasses['transition']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:bg-slate-100']} */ ;
/** @type {__VLS_StyleScopedClasses['hover:text-slate-700']} */ ;
/** @type {__VLS_StyleScopedClasses['mb-3']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['flex-wrap']} */ ;
/** @type {__VLS_StyleScopedClasses['gap-2']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-md']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-100']} */ ;
/** @type {__VLS_StyleScopedClasses['px-2']} */ ;
/** @type {__VLS_StyleScopedClasses['py-1']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-600']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-md']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-100']} */ ;
/** @type {__VLS_StyleScopedClasses['px-2']} */ ;
/** @type {__VLS_StyleScopedClasses['py-1']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-600']} */ ;
/** @type {__VLS_StyleScopedClasses['space-y-2']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['font-medium']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-500']} */ ;
/** @type {__VLS_StyleScopedClasses['whitespace-pre-wrap']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-lg']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-50']} */ ;
/** @type {__VLS_StyleScopedClasses['p-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['leading-relaxed']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-700']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-lg']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-dashed']} */ ;
/** @type {__VLS_StyleScopedClasses['border-slate-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-50']} */ ;
/** @type {__VLS_StyleScopedClasses['p-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-500']} */ ;
/** @type {__VLS_StyleScopedClasses['mt-3']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-lg']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-violet-200']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-violet-50/60']} */ ;
/** @type {__VLS_StyleScopedClasses['p-2.5']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['font-medium']} */ ;
/** @type {__VLS_StyleScopedClasses['text-violet-700']} */ ;
/** @type {__VLS_StyleScopedClasses['mt-1']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['text-violet-700']} */ ;
/** @type {__VLS_StyleScopedClasses['mt-1']} */ ;
/** @type {__VLS_StyleScopedClasses['whitespace-pre-wrap']} */ ;
/** @type {__VLS_StyleScopedClasses['text-xs']} */ ;
/** @type {__VLS_StyleScopedClasses['leading-relaxed']} */ ;
/** @type {__VLS_StyleScopedClasses['text-slate-700']} */ ;
/** @type {__VLS_StyleScopedClasses['fixed']} */ ;
/** @type {__VLS_StyleScopedClasses['inset-0']} */ ;
/** @type {__VLS_StyleScopedClasses['z-50']} */ ;
/** @type {__VLS_StyleScopedClasses['flex']} */ ;
/** @type {__VLS_StyleScopedClasses['items-center']} */ ;
/** @type {__VLS_StyleScopedClasses['justify-center']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-950/30']} */ ;
/** @type {__VLS_StyleScopedClasses['backdrop-blur-[2px]']} */ ;
/** @type {__VLS_StyleScopedClasses['rounded-2xl']} */ ;
/** @type {__VLS_StyleScopedClasses['border']} */ ;
/** @type {__VLS_StyleScopedClasses['border-white/20']} */ ;
/** @type {__VLS_StyleScopedClasses['bg-slate-900/90']} */ ;
/** @type {__VLS_StyleScopedClasses['px-8']} */ ;
/** @type {__VLS_StyleScopedClasses['py-6']} */ ;
/** @type {__VLS_StyleScopedClasses['text-center']} */ ;
/** @type {__VLS_StyleScopedClasses['text-white']} */ ;
/** @type {__VLS_StyleScopedClasses['shadow-2xl']} */ ;
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
/** @type {__VLS_StyleScopedClasses['tracking-wide']} */ ;
var __VLS_dollars;
const __VLS_self = (await import('vue')).defineComponent({
    setup() {
        return {
            markdown: markdown,
            concept: concept,
            userId: userId,
            maxDepth: maxDepth,
            isLoading: isLoading,
            isNavigating: isNavigating,
            statusText: statusText,
            importedFileName: importedFileName,
            graphRoot: graphRoot,
            selectedNodeDetail: selectedNodeDetail,
            isExplaining: isExplaining,
            canGenerate: canGenerate,
            canNavigate: canNavigate,
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
