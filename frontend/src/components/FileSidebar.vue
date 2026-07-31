<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ChevronDown, ChevronRight, FileText, Folder, FolderPlus, MoreHorizontal, RefreshCw, Upload } from "lucide-vue-next";
import type { FileGroup, UserFile } from "../api/graph";

const props = defineProps<{
  width: number;
  loggedInUserId: string;
  files: UserFile[];
  fileGroups: FileGroup[];
  selectedFileId: string;
  selectedFileGroupId: string;
  newGroupName: string;
  menuOpen: string;
  isRefreshing?: boolean;
}>();

const emit = defineEmits<{
  "update:newGroupName": [value: string];
  "update:menuOpen": [value: string];
  loginRequired: [];
  uploadFile: [event: Event];
  uploadFileGroup: [event: Event];
  createGroup: [];
  selectFile: [id: string];
  selectFileGroup: [id: string];
  togglePin: [type: "file" | "group", id: string];
  rename: [type: "file" | "group", id: string, name: string];
  deleteFile: [id: string];
  deleteGroup: [id: string];
  addToGroup: [fileId: string];
  refresh: [];
}>();

const expandedGroupIds = ref<Set<string>>(new Set());
const sortedGroups = computed(() =>
  [...props.fileGroups].sort((a, b) => Number(Boolean(b.pinned)) - Number(Boolean(a.pinned)) || a.name.localeCompare(b.name)),
);
const standaloneFiles = computed(() =>
  props.files
    .filter((file) => !file.file_group_id)
    .sort((a, b) => Number(Boolean(b.pinned)) - Number(Boolean(a.pinned)) || a.name.localeCompare(b.name)),
);

function filesInGroup(groupId: string) {
  return props.files
    .filter((file) => file.file_group_id === groupId)
    .sort((a, b) => Number(Boolean(b.pinned)) - Number(Boolean(a.pinned)) || a.name.localeCompare(b.name));
}

function toggleGroup(groupId: string) {
  const next = new Set(expandedGroupIds.value);
  if (next.has(groupId)) next.delete(groupId);
  else next.add(groupId);
  expandedGroupIds.value = next;
}

function selectGroup(groupId: string) {
  expandedGroupIds.value = new Set(expandedGroupIds.value).add(groupId);
  emit("selectFileGroup", groupId);
}

watch(
  () => props.selectedFileGroupId,
  (groupId) => {
    if (groupId) expandedGroupIds.value = new Set(expandedGroupIds.value).add(groupId);
  },
  { immediate: true },
);
</script>

<template>
  <aside
    :style="{ width: `${width}px` }"
    class="flex h-full shrink-0 flex-col overflow-hidden border-r border-gray-200 bg-white"
  >
    <div class="flex items-center justify-between border-b border-gray-200 px-4 py-3">
      <div>
        <h2 class="text-base font-semibold text-slate-900">文件</h2>
        <p class="text-[13px] text-slate-500">课程笔记与文件组</p>
      </div>
    </div>

    <div class="space-y-2 border-b border-gray-100 px-3 py-3">
      <div class="flex gap-2">
        <label
          v-if="loggedInUserId"
          class="inline-flex h-11 flex-1 cursor-pointer items-center justify-center gap-1.5 rounded-xl bg-indigo-600 text-sm font-medium text-white transition duration-200 hover:bg-indigo-500"
        >
          <Upload class="h-4 w-4" />
          上传
          <input class="hidden" type="file" accept=".md,text/markdown" @change="emit('uploadFile', $event)" />
        </label>
        <button
          v-else
          type="button"
          class="inline-flex h-11 flex-1 items-center justify-center gap-1.5 rounded-xl bg-indigo-600 text-sm font-medium text-white transition duration-200 hover:bg-indigo-500"
          @click="emit('loginRequired')"
        >
          <Upload class="h-4 w-4" />
          上传
        </button>

        <label
          v-if="loggedInUserId"
          class="inline-flex h-11 flex-1 cursor-pointer items-center justify-center gap-1.5 rounded-xl border border-gray-200 bg-white text-sm font-medium text-slate-700 transition duration-200 hover:bg-gray-100"
        >
          <FolderPlus class="h-4 w-4" />
          文件组
          <input
            class="hidden"
            type="file"
            accept=".md,text/markdown"
            multiple
            @change="emit('uploadFileGroup', $event)"
          />
        </label>
        <button
          v-else
          type="button"
          class="inline-flex h-11 flex-1 items-center justify-center gap-1.5 rounded-xl border border-gray-200 bg-white text-sm font-medium text-slate-700 transition duration-200 hover:bg-gray-100"
          @click="emit('loginRequired')"
        >
          <FolderPlus class="h-4 w-4" />
          文件组
        </button>
      </div>
    </div>

    <div class="border-b border-gray-100 px-3 py-3">
      <div class="mb-2 flex items-center gap-2">
        <input
          :value="newGroupName"
          :disabled="!loggedInUserId"
          class="h-11 flex-1 rounded-xl border border-gray-200 px-3 text-sm outline-none transition duration-200 focus:border-indigo-500 disabled:cursor-pointer disabled:bg-slate-50"
          :placeholder="loggedInUserId ? '新建文件组…' : '登录后创建文件组'"
          @click="!loggedInUserId && emit('loginRequired')"
          @input="emit('update:newGroupName', ($event.target as HTMLInputElement).value)"
          @keyup.enter="emit('createGroup')"
        />
        <button
          type="button"
          :disabled="loggedInUserId ? !newGroupName.trim() : false"
          class="inline-flex h-11 w-11 items-center justify-center rounded-xl bg-violet-50 text-violet-700 transition duration-200 hover:bg-violet-100 disabled:cursor-not-allowed disabled:opacity-40"
          :title="loggedInUserId ? '创建文件组' : '登录后创建文件组'"
          @click="loggedInUserId ? emit('createGroup') : emit('loginRequired')"
        >
          +
        </button>
      </div>

      <div class="max-h-[42vh] space-y-1 overflow-y-auto">
        <p v-if="sortedGroups.length === 0" class="py-3 text-center text-[13px] text-slate-400">
          暂无文件组
        </p>
        <div v-for="g in sortedGroups" :key="g.id" class="relative">
          <div
            class="flex cursor-pointer items-center gap-1 rounded-xl px-2 py-2 text-sm transition duration-200"
            :class="selectedFileGroupId === g.id ? 'bg-violet-50 font-medium text-violet-800' : 'text-slate-700 hover:bg-gray-100'"
            @click="selectGroup(g.id)"
          >
            <button type="button" class="rounded p-0.5 text-slate-400" @click.stop="toggleGroup(g.id)">
              <ChevronDown v-if="expandedGroupIds.has(g.id)" class="h-4 w-4" />
              <ChevronRight v-else class="h-4 w-4" />
            </button>
            <Folder class="h-4 w-4 shrink-0" :class="g.pinned ? 'text-violet-600' : 'text-slate-400'" />
            <span class="min-w-0 flex-1 truncate">{{ g.name }}</span>
            <span class="text-[12px] text-slate-400">{{ filesInGroup(g.id).length }}</span>
            <button
              type="button"
              class="rounded-lg p-1 text-slate-400 hover:bg-white hover:text-slate-600"
              @click.stop="emit('update:menuOpen', menuOpen === `group:${g.id}` ? '' : `group:${g.id}`)"
            >
              <MoreHorizontal class="h-4 w-4" />
            </button>
          </div>

          <div v-if="expandedGroupIds.has(g.id)" class="ml-6 mt-1 space-y-0.5 border-l border-violet-100 pl-2">
            <p v-if="filesInGroup(g.id).length === 0" class="px-2 py-2 text-[12px] text-slate-400">空文件组</p>
            <div v-for="f in filesInGroup(g.id)" :key="f.id" class="relative">
              <div
                class="flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-[13px] transition"
                :class="selectedFileId === f.id ? 'bg-indigo-50 font-medium text-indigo-700' : 'text-slate-600 hover:bg-gray-50'"
                @click="emit('selectFile', f.id)"
              >
                <FileText class="h-3.5 w-3.5 shrink-0" />
                <span class="min-w-0 flex-1 truncate">{{ f.name }}</span>
                <button class="rounded p-1 text-slate-400 hover:bg-white" @click.stop="emit('update:menuOpen', menuOpen === `file:${f.id}` ? '' : `file:${f.id}`)">
                  <MoreHorizontal class="h-3.5 w-3.5" />
                </button>
              </div>
              <div v-if="menuOpen === `file:${f.id}`" class="absolute right-1 top-8 z-40 min-w-[130px] rounded-xl border border-gray-200 bg-white py-1 shadow-md" @click.stop>
                <button class="w-full px-3 py-2 text-left text-sm text-slate-600 hover:bg-gray-50" @click="emit('togglePin', 'file', f.id)">{{ f.pinned ? "取消置顶" : "置顶" }}</button>
                <button class="w-full px-3 py-2 text-left text-sm text-slate-600 hover:bg-gray-50" @click="emit('rename', 'file', f.id, f.name)">改名</button>
                <button class="w-full px-3 py-2 text-left text-sm text-slate-600 hover:bg-gray-50" @click="emit('addToGroup', f.id)">移动到其他组</button>
                <button class="w-full px-3 py-2 text-left text-sm text-red-500 hover:bg-red-50" @click="emit('deleteFile', f.id)">删除</button>
              </div>
            </div>
          </div>

          <div v-if="menuOpen === `group:${g.id}`" class="absolute right-2 top-10 z-30 min-w-[120px] rounded-xl border border-gray-200 bg-white py-1 shadow-md" @click.stop>
            <button class="w-full px-3 py-2 text-left text-sm text-slate-600 hover:bg-gray-50" @click="emit('togglePin', 'group', g.id)">{{ g.pinned ? "取消置顶" : "置顶" }}</button>
            <button class="w-full px-3 py-2 text-left text-sm text-slate-600 hover:bg-gray-50" @click="emit('rename', 'group', g.id, g.name)">改名</button>
            <button class="w-full px-3 py-2 text-left text-sm text-red-500 hover:bg-red-50" @click="emit('deleteGroup', g.id)">删除</button>
          </div>
        </div>
      </div>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto px-3 py-3">
      <p class="mb-2 px-1 text-[13px] text-slate-400">独立文件</p>
      <div class="space-y-1">
        <p v-if="standaloneFiles.length === 0" class="py-3 text-center text-[13px] text-slate-400">暂无独立文件</p>
        <div v-for="f in standaloneFiles" :key="f.id" class="relative">
          <div
            class="flex cursor-pointer items-center gap-2 rounded-xl px-2.5 py-2 text-sm transition duration-200"
            :class="
              selectedFileId === f.id
                ? 'bg-indigo-50 font-medium text-indigo-800'
                : 'text-slate-700 hover:bg-gray-100'
            "
            @click="emit('selectFile', f.id)"
          >
            <span class="min-w-0 flex-1 truncate">{{ f.pinned ? "📌 " : "" }}{{ f.name }}</span>
            <button
              type="button"
              class="rounded-lg p-1 text-slate-400 hover:bg-white hover:text-slate-600"
              @click.stop="emit('update:menuOpen', menuOpen === `file:${f.id}` ? '' : `file:${f.id}`)"
            >
              <MoreHorizontal class="h-4 w-4" />
            </button>
          </div>
          <div
            v-if="menuOpen === `file:${f.id}`"
            class="absolute right-2 top-10 z-30 min-w-[130px] rounded-xl border border-gray-200 bg-white py-1 shadow-md"
            @click.stop
          >
            <button
              class="w-full px-3 py-2 text-left text-sm text-slate-600 hover:bg-gray-50"
              @click="emit('togglePin', 'file', f.id)"
            >
              {{ f.pinned ? "取消置顶" : "置顶" }}
            </button>
            <button
              class="w-full px-3 py-2 text-left text-sm text-slate-600 hover:bg-gray-50"
              @click="emit('rename', 'file', f.id, f.name)"
            >
              改名
            </button>
            <button
              class="w-full px-3 py-2 text-left text-sm text-slate-600 hover:bg-gray-50"
              @click="emit('addToGroup', f.id)"
            >
              加入文件组
            </button>
            <button
              class="w-full px-3 py-2 text-left text-sm text-red-500 hover:bg-red-50"
              @click="emit('deleteFile', f.id)"
            >
              删除
            </button>
          </div>
        </div>
      </div>

    </div>

    <div class="border-t border-gray-200 p-3">
      <button
        type="button"
        :disabled="isRefreshing"
        class="inline-flex h-11 w-full items-center justify-center gap-2 rounded-xl bg-slate-100 text-sm font-medium text-slate-600 transition duration-200 hover:bg-slate-200 disabled:cursor-not-allowed disabled:opacity-60"
        @click="emit('refresh')"
      >
        <RefreshCw class="h-4 w-4" :class="{ 'animate-spin': isRefreshing }" />
        {{ isRefreshing ? "刷新中…" : "刷新" }}
      </button>
    </div>
  </aside>
</template>
