<script setup lang="ts">
/**
 * 轻量提示条（toast）
 *
 * 用途：反馈「查询未命中」「深度被截断」这类**不需要用户决策**的信息。
 * 刻意不做成需要手动关闭的弹窗——这类提示只是告知，不该打断操作，
 * 因此到时间自动淡出。
 *
 * 设计要点：
 * - 自动淡出，无需点击（但仍提供手动关闭，照顾想立刻清掉的场景）
 * - 多条并存时纵向堆叠，各自独立计时
 * - 用淡入 + 上移的过渡，避免生硬出现
 */
import { computed } from "vue";

export interface ToastItem {
  id: number;
  message: string;
  /** 提示类型，决定配色 */
  type?: "info" | "warn" | "error";
  /** 可选的补充说明（如「已按上限 12 计算」） */
  detail?: string;
}

const props = defineProps<{
  toasts: ToastItem[];
  /** 每条提示的停留毫秒数，默认 4.2 秒 */
  duration?: number;
}>();

const emit = defineEmits<{
  (e: "dismiss", id: number): void;
}>();

const toneClass = computed(() => (type?: ToastItem["type"]) => {
  switch (type) {
    case "error":
      return "border-rose-400/60 bg-rose-950/90 text-rose-100";
    case "warn":
      return "border-amber-400/60 bg-amber-950/90 text-amber-100";
    default:
      return "border-indigo-400/60 bg-slate-900/95 text-slate-100";
  }
});
</script>

<template>
  <!-- 固定在顶部居中，不遮挡图谱主要操作区 -->
  <div class="pointer-events-none fixed left-1/2 top-4 z-[100] flex w-[min(560px,calc(100vw-2rem))] -translate-x-1/2 flex-col gap-2">
    <TransitionGroup
      name="toast"
      tag="div"
      class="flex flex-col gap-2"
    >
      <div
        v-for="t in props.toasts"
        :key="t.id"
        :class="['pointer-events-auto flex items-start gap-2.5 rounded-xl border px-3.5 py-2.5 shadow-lg backdrop-blur', toneClass(t.type)]"
        role="status"
        aria-live="polite"
      >
        <div class="min-w-0 flex-1">
          <p class="text-[12.5px] leading-relaxed">{{ t.message }}</p>
          <p v-if="t.detail" class="mt-0.5 text-[11px] leading-relaxed opacity-80">{{ t.detail }}</p>
        </div>
        <button
          class="mt-0.5 shrink-0 text-[15px] leading-none opacity-60 transition hover:opacity-100"
          aria-label="关闭提示"
          @click="emit('dismiss', t.id)"
        >
          ×
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
/* 淡入 + 轻微上移，避免提示突兀出现 */
.toast-enter-active,
.toast-leave-active {
  transition: opacity 0.28s ease, transform 0.28s ease;
}
.toast-enter-from {
  opacity: 0;
  transform: translateY(-8px);
}
.toast-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}
.toast-leave-active {
  position: absolute;
  width: 100%;
}
</style>
