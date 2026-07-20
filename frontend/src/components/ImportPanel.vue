<script setup lang="ts">
import { FileUp, Sparkles } from "lucide-vue-next";

defineProps<{
  markdown: string;
  importedFileName: string;
  userId: string;
  loggedInUserId: string;
  isLoading: boolean;
  canGenerate: boolean;
}>();

const emit = defineEmits<{
  "update:markdown": [value: string];
  "update:userId": [value: string];
  login: [];
  importFile: [event: Event];
  generate: [];
}>();
</script>

<template>
  <section class="shrink-0 rounded-[18px] border border-gray-200 bg-white p-4 shadow-sm">
    <div class="mb-3 flex items-start justify-between gap-2">
      <div class="min-w-0">
        <h2 class="text-base font-semibold text-slate-900">导入学习笔记</h2>
        <p class="mt-0.5 text-[13px] text-slate-500">上传 Markdown，生成图谱</p>
      </div>
      <label
        class="inline-flex h-11 shrink-0 cursor-pointer items-center gap-1.5 rounded-xl border border-gray-200 bg-white px-3 text-sm font-medium text-slate-700 transition duration-200 hover:bg-gray-100"
      >
        <FileUp class="h-4 w-4" />
        导入
        <input class="hidden" type="file" accept=".md,text/markdown" @change="emit('importFile', $event)" />
      </label>
    </div>

    <p v-if="importedFileName" class="mb-2 truncate text-[13px] text-indigo-600">已导入：{{ importedFileName }}</p>

    <textarea
      :value="markdown"
      class="h-20 w-full resize-none rounded-xl border border-gray-200 bg-slate-50 p-2.5 text-sm text-slate-700 outline-none transition duration-200 placeholder:text-slate-400 focus:border-indigo-500"
      placeholder="粘贴 Markdown 笔记…"
      @input="emit('update:markdown', ($event.target as HTMLTextAreaElement).value)"
    />

    <div class="mt-3 flex items-center gap-2">
      <input
        :value="userId"
        class="h-11 w-24 shrink-0 rounded-xl border border-gray-200 px-2.5 text-sm outline-none transition duration-200 focus:border-indigo-500"
        placeholder="user_id"
        @input="emit('update:userId', ($event.target as HTMLInputElement).value)"
      />
      <button
        type="button"
        class="h-11 shrink-0 rounded-xl border border-gray-200 bg-white px-3 text-sm font-medium text-slate-700 transition duration-200 hover:bg-gray-100"
        @click="emit('login')"
      >
        登录
      </button>
      <button
        type="button"
        :disabled="!canGenerate || isLoading"
        class="inline-flex h-11 min-w-0 flex-1 items-center justify-center gap-1.5 rounded-xl bg-indigo-600 px-3 text-sm font-medium text-white transition duration-200 hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50"
        @click="emit('generate')"
      >
        <Sparkles class="h-4 w-4 shrink-0" />
        <span class="truncate">{{ isLoading ? "诊断中…" : "生成图谱" }}</span>
      </button>
    </div>

    <p v-if="loggedInUserId" class="mt-2 truncate text-[13px] text-emerald-600">当前用户：{{ loggedInUserId }}</p>
  </section>
</template>
