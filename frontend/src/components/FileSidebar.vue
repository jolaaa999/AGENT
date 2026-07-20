<script setup lang="ts">
import { FolderPlus, MoreHorizontal, RefreshCw, Upload } from "lucide-vue-next";
import type { FileGroup, UserFile } from "../api/graph";

defineProps<{
  width: number;
  loggedInUserId: string;
  files: UserFile[];
  fileGroups: FileGroup[];
  selectedFileId: string;
  selectedFileGroupId: string;
  newGroupName: string;
  menuOpen: string;
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
          class="h-11 flex-1 rounded-xl border border-gray-200 px-3 text-sm outline-none transition duration-200 focus:border-indigo-500"
          placeholder="新建文件组…"
          @input="emit('update:newGroupName', ($event.target as HTMLInputElement).value)"
          @keyup.enter="emit('createGroup')"
        />
        <button
          type="button"
          class="inline-flex h-11 w-11 items-center justify-center rounded-xl bg-violet-50 text-violet-700 transition duration-200 hover:bg-violet-100"
          @click="emit('createGroup')"
        >
          +
        </button>
      </div>

      <div class="max-h-[22vh] space-y-1 overflow-y-auto">
        <div v-for="g in fileGroups" :key="g.id" class="relative">
          <div
            class="flex cursor-pointer items-center gap-2 rounded-xl px-2.5 py-2 text-sm transition duration-200"
            :class="
              selectedFileGroupId === g.id
                ? 'bg-violet-50 font-medium text-violet-800'
                : 'text-slate-700 hover:bg-gray-100'
            "
            @click="emit('selectFileGroup', g.id)"
          >
            <span class="min-w-0 flex-1 truncate">{{ g.pinned ? "📌 " : "" }}{{ g.name }}</span>
            <span class="text-[13px] text-slate-400">({{ g.file_ids?.length ?? 0 }})</span>
            <button
              type="button"
              class="rounded-lg p-1 text-slate-400 hover:bg-white hover:text-slate-600"
              @click.stop="emit('update:menuOpen', menuOpen === g.id ? '' : g.id)"
            >
              <MoreHorizontal class="h-4 w-4" />
            </button>
          </div>
          <div
            v-if="menuOpen === g.id"
            class="absolute right-2 top-10 z-30 min-w-[120px] rounded-xl border border-gray-200 bg-white py-1 shadow-md"
            @click.stop
          >
            <button
              class="w-full px-3 py-2 text-left text-sm text-slate-600 hover:bg-gray-50"
              @click="emit('togglePin', 'group', g.id)"
            >
              {{ g.pinned ? "取消置顶" : "置顶" }}
            </button>
            <button
              class="w-full px-3 py-2 text-left text-sm text-slate-600 hover:bg-gray-50"
              @click="emit('rename', 'group', g.id, g.name)"
            >
              改名
            </button>
            <button
              class="w-full px-3 py-2 text-left text-sm text-red-500 hover:bg-red-50"
              @click="emit('deleteGroup', g.id)"
            >
              删除
            </button>
          </div>
        </div>
      </div>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto px-3 py-3">
      <p class="mb-2 px-1 text-[13px] text-slate-400">独立文件</p>
      <div class="space-y-1">
        <div v-for="f in files.filter((x) => !x.file_group_id)" :key="f.id" class="relative">
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
              @click.stop="emit('update:menuOpen', menuOpen === f.id ? '' : f.id)"
            >
              <MoreHorizontal class="h-4 w-4" />
            </button>
          </div>
          <div
            v-if="menuOpen === f.id"
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

      <template v-if="selectedFileGroupId">
        <p class="mb-2 mt-4 px-1 text-[13px] text-violet-500">组内文件</p>
        <div
          v-for="f in files.filter((x) => x.file_group_id === selectedFileGroupId)"
          :key="f.id"
          class="cursor-pointer rounded-xl px-2.5 py-2 text-sm text-violet-700 transition duration-200 hover:bg-violet-50"
          :class="selectedFileId === f.id ? 'bg-violet-100 font-medium' : ''"
          @click="emit('selectFile', f.id)"
        >
          {{ f.name }}
        </div>
      </template>
    </div>

    <div class="border-t border-gray-200 p-3">
      <button
        type="button"
        class="inline-flex h-11 w-full items-center justify-center gap-2 rounded-xl bg-slate-100 text-sm font-medium text-slate-600 transition duration-200 hover:bg-slate-200"
        @click="emit('refresh')"
      >
        <RefreshCw class="h-4 w-4" />
        刷新
      </button>
    </div>
  </aside>
</template>
