<script setup lang="ts">
/**
 * AI 工作区面板
 *
 * 展示 AI 可以操作的文件（沙箱目录），并支持：
 * - 手动把已上传的笔记同步进工作区
 * - 查看 / 下载 AI 修改后的文件
 *
 * 设计约束：这里是右侧栏的第 4 个面板，而 AI 学习导师（AiChatPanel）
 * 需要用 flex-1 占满剩余高度，因此本面板必须：
 *   1. 默认折叠，只占一行高度，不与聊天区争抢空间
 *   2. 用 shrink-0，展开时也不挤压其他面板
 *   3. 沿用浅色主题，与 ImportPanel / LearningNavPanel 保持一致
 */
import { computed, onMounted, ref, watch } from "vue";
import { ChevronDown, FolderOpen } from "lucide-vue-next";
import {
  listWorkspaceFiles,
  readWorkspaceFile,
  workspaceDownloadUrl,
  type WorkspaceFile,
} from "../api/graph";

const props = defineProps<{
  userId: string;
  /** 已入库的文件列表，用于提供「同步到工作区」入口 */
  libraryFiles?: { id: string; name: string }[];
}>();

const emit = defineEmits<{
  (e: "sync-file", fileId: string): void;
  (e: "notify", message: string): void;
}>();

const files = ref<WorkspaceFile[]>([]);
const loading = ref(false);
const errorText = ref("");
const expanded = ref(false);
const previewPath = ref("");
const previewContent = ref("");
const previewLoading = ref(false);

const hasFiles = computed(() => files.value.length > 0);

/** 可编辑的 Markdown 文档（AI 可读写、可下载改后稿） */
const docs = computed(() => files.value.filter((f) => !f.is_original));
/** 上传时的原始文件（docx/pdf/图片等，字节级等于用户上传的那份） */
const originals = computed(() => files.value.filter((f) => f.is_original));

const hasDocs = computed(() => docs.value.length > 0);
const hasOriginals = computed(() => originals.value.length > 0);

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

async function refresh() {
  if (!props.userId) return;
  loading.value = true;
  errorText.value = "";
  try {
    const resp = await listWorkspaceFiles(props.userId);
    files.value = resp.files ?? [];
  } catch (err) {
    errorText.value = err instanceof Error ? err.message : "读取工作区失败";
    files.value = [];
  } finally {
    loading.value = false;
  }
}

async function openPreview(path: string) {
  previewPath.value = path;
  previewLoading.value = true;
  previewContent.value = "";
  try {
    const resp = await readWorkspaceFile(path, props.userId);
    previewContent.value = resp.content || "（空文件）";
  } catch (err) {
    previewContent.value = `读取失败：${err instanceof Error ? err.message : String(err)}`;
  } finally {
    previewLoading.value = false;
  }
}

function closePreview() {
  previewPath.value = "";
  previewContent.value = "";
}

function download(path: string) {
  window.open(workspaceDownloadUrl(path, props.userId), "_blank");
  emit("notify", `已开始下载 ${path}`);
}

function downloadAll() {
  if (!hasFiles.value) return;
  // 文档与原件都下载，确保用户拿得到「改后稿」和「未改动的原件」
  files.value.forEach((f, index) => {
    window.setTimeout(() => {
      window.open(workspaceDownloadUrl(f.name, props.userId), "_blank");
    }, index * 300);
  });
}

function syncFromLibrary(fileId: string) {
  emit("sync-file", fileId);
}

defineExpose({ refresh });

onMounted(() => {
  void refresh();
});
watch(() => props.userId, () => void refresh());
// 首次出现文件时自动展开一次，让用户能看到结果
watch(hasFiles, (now, before) => {
  if (now && !before) expanded.value = true;
});
</script>

<template>
  <!-- shrink-0 + 可折叠：保证不侵占 AI 学习导师的剩余高度 -->
  <section data-panel="workspace" class="shrink-0 rounded-[18px] border border-gray-200 bg-white shadow-sm">
    <!-- 折叠栏（始终可见的一行） -->
    <button
      type="button"
      class="flex w-full items-center justify-between gap-2 px-4 py-3 text-left"
      @click="expanded = !expanded"
    >
      <div class="flex min-w-0 items-center gap-2">
        <FolderOpen class="h-4 w-4 shrink-0 text-slate-400" />
        <div class="min-w-0">
          <p class="text-[13px] font-semibold text-slate-900">AI 工作区</p>
          <p class="mt-0.5 truncate text-[12px] text-slate-500">
            <template v-if="hasFiles">
              {{ docs.length }} 份文档<template v-if="hasOriginals"> · {{ originals.length }} 个原件</template>
            </template>
            <template v-else>AI 在云端操作的文件目录</template>
          </p>
        </div>
      </div>
      <ChevronDown
        class="h-4 w-4 shrink-0 text-slate-400 transition-transform duration-200"
        :class="expanded ? 'rotate-180' : ''"
      />
    </button>

    <!-- 展开内容 -->
    <div v-if="expanded" class="border-t border-gray-100 px-4 pb-3 pt-2.5">
      <p v-if="errorText" class="mb-2 rounded-lg border border-red-200 bg-red-50 px-2 py-1.5 text-[12px] text-red-600">
        {{ errorText }}
      </p>

      <div v-if="!hasFiles && !loading" class="py-2 text-center">
        <p class="text-[12px] text-slate-500">工作区还是空的</p>
        <p class="mt-0.5 text-[11px] leading-relaxed text-slate-400">
          上传笔记后会自动同步，也可在下方选择已有文件同步
        </p>
      </div>

      <template v-else>
        <!-- 可编辑文档：AI 读写的就是这些 Markdown，下载得到「改后稿」 -->
        <div v-if="hasDocs">
          <p class="mb-1 flex items-center gap-1 text-[11px] font-medium text-slate-500">
            可编辑文档
            <span class="font-normal text-slate-400">（AI 修改后可下载改后稿）</span>
          </p>
          <ul class="max-h-[120px] space-y-1 overflow-y-auto">
            <li
              v-for="f in docs"
              :key="f.name"
              class="flex items-center gap-2 rounded-lg px-2 py-1.5 transition hover:bg-gray-50"
            >
              <div class="min-w-0 flex-1">
                <p class="truncate text-[12px] text-slate-700" :title="f.name">{{ f.name }}</p>
                <p class="text-[10px] text-slate-400">{{ formatSize(f.size) }}</p>
              </div>
              <button
                class="shrink-0 rounded-lg border border-gray-200 px-2 py-0.5 text-[11px] text-slate-600 transition hover:border-slate-400 hover:text-slate-900"
                @click="openPreview(f.name)"
              >
                预览
              </button>
              <button
                class="shrink-0 rounded-lg bg-indigo-600 px-2 py-0.5 text-[11px] text-white transition hover:bg-indigo-500"
                @click="download(f.name)"
              >
                下载
              </button>
            </li>
          </ul>
        </div>

        <!-- 原件：上传时归档的原始文件，未经过任何转换 -->
        <div v-if="hasOriginals" class="mt-2.5">
          <p class="mb-1 flex items-center gap-1 text-[11px] font-medium text-slate-500">
            原始文件
            <span class="font-normal text-slate-400">（与上传时一致，未经转换）</span>
          </p>
          <ul class="max-h-[120px] space-y-1 overflow-y-auto">
            <li
              v-for="f in originals"
              :key="f.name"
              class="flex items-center gap-2 rounded-lg bg-amber-50/60 px-2 py-1.5 transition hover:bg-amber-50"
            >
              <div class="min-w-0 flex-1">
                <p class="truncate text-[12px] text-slate-700" :title="f.name">
                  {{ f.name.replace(/^original\//, "") }}
                </p>
                <p class="text-[10px] text-slate-400">{{ f.kind || "文件" }} · {{ formatSize(f.size) }}</p>
              </div>
              <button
                class="shrink-0 rounded-lg bg-amber-600 px-2 py-0.5 text-[11px] text-white transition hover:bg-amber-500"
                @click="download(f.name)"
              >
                下载原件
              </button>
            </li>
          </ul>
        </div>
      </template>

      <div class="mt-2.5 flex items-center gap-2">
        <button
          v-if="hasFiles"
          type="button"
          class="shrink-0 rounded-lg border border-gray-200 px-2 py-1 text-[11px] text-slate-600 transition hover:border-slate-400 hover:text-slate-900"
          @click="downloadAll"
        >
          全部下载
        </button>
        <button
          type="button"
          class="shrink-0 rounded-lg border border-gray-200 px-2 py-1 text-[11px] text-slate-600 transition hover:border-slate-400 hover:text-slate-900 disabled:opacity-50"
          :disabled="loading"
          @click="refresh"
        >
          {{ loading ? "刷新中…" : "刷新" }}
        </button>
      </div>

      <div v-if="props.libraryFiles && props.libraryFiles.length" class="mt-2.5">
        <p class="mb-1 text-[11px] text-slate-500">从已有文件同步到工作区</p>
        <select
          class="w-full rounded-lg border border-gray-200 bg-white px-2 py-1.5 text-[12px] text-slate-700 outline-none transition focus:border-indigo-500"
          @change="(e) => { const v = (e.target as HTMLSelectElement).value; if (v) { syncFromLibrary(v); (e.target as HTMLSelectElement).value = ''; } }"
        >
          <option value="">选择文件…</option>
          <option v-for="lf in props.libraryFiles" :key="lf.id" :value="lf.id">
            {{ lf.name }}
          </option>
        </select>
      </div>
    </div>

    <!-- 预览浮层 -->
    <div
      v-if="previewPath"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      @click.self="closePreview"
    >
      <div class="flex max-h-[80vh] w-full max-w-2xl flex-col rounded-[18px] border border-gray-200 bg-white shadow-xl">
        <div class="flex items-center justify-between gap-2 border-b border-gray-100 px-4 py-3">
          <p class="truncate text-[13px] font-medium text-slate-900">{{ previewPath }}</p>
          <div class="flex shrink-0 items-center gap-2">
            <button
              type="button"
              class="rounded-lg bg-indigo-600 px-2.5 py-1 text-[12px] text-white transition hover:bg-indigo-500"
              @click="download(previewPath)"
            >
              下载
            </button>
            <button class="text-[18px] leading-none text-slate-400 transition hover:text-slate-700" @click="closePreview">×</button>
          </div>
        </div>
        <pre class="min-h-0 flex-1 overflow-auto whitespace-pre-wrap break-words px-4 py-3 text-[12px] leading-relaxed text-slate-700">{{ previewLoading ? "读取中…" : previewContent }}</pre>
      </div>
    </div>
  </section>
</template>
