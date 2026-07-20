<script setup lang="ts">
import { Compass, RotateCcw } from "lucide-vue-next";

defineProps<{
  concept: string;
  maxDepth: number;
  canNavigate: boolean;
  isLoading: boolean;
  isNavigating: boolean;
}>();

const emit = defineEmits<{
  "update:concept": [value: string];
  "update:maxDepth": [value: number];
  navigate: [];
  reset: [];
}>();
</script>

<template>
  <section class="shrink-0 rounded-[18px] border border-gray-200 bg-white p-4 shadow-sm">
    <div class="mb-3">
      <h2 class="text-base font-semibold text-slate-900">逆向学习导航</h2>
      <p class="mt-0.5 text-[13px] text-slate-500">输入目标概念，回溯前置知识</p>
    </div>

    <div class="flex gap-2">
      <input
        :value="concept"
        class="h-11 min-w-0 flex-1 rounded-xl border border-gray-200 px-3 text-sm outline-none transition duration-200 placeholder:text-slate-400 focus:border-indigo-500"
        placeholder="目标概念，例如：Transformer"
        @input="emit('update:concept', ($event.target as HTMLInputElement).value)"
        @keyup.enter="emit('navigate')"
      />
      <input
        :value="maxDepth"
        type="number"
        min="1"
        max="6"
        class="h-11 w-14 shrink-0 rounded-xl border border-gray-200 px-1 text-center text-sm outline-none transition duration-200 focus:border-indigo-500"
        title="最大深度"
        @input="emit('update:maxDepth', Number(($event.target as HTMLInputElement).value) || 1)"
      />
    </div>

    <div class="mt-3 flex gap-2">
      <button
        type="button"
        :disabled="!canNavigate || isLoading"
        class="inline-flex h-11 min-w-0 flex-1 items-center justify-center gap-1.5 rounded-xl bg-indigo-600 px-3 text-sm font-medium text-white transition duration-200 hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50"
        @click="emit('navigate')"
      >
        <Compass class="h-4 w-4 shrink-0" />
        开始导航
      </button>
      <button
        v-if="isNavigating"
        type="button"
        class="inline-flex h-11 shrink-0 items-center gap-1.5 rounded-xl border border-gray-200 bg-white px-3 text-sm font-medium text-slate-700 transition duration-200 hover:bg-gray-100"
        @click="emit('reset')"
      >
        <RotateCcw class="h-4 w-4" />
        退出
      </button>
    </div>
  </section>
</template>
