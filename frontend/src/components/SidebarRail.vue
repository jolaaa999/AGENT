<script setup lang="ts">
/**
 * 折叠态侧栏图标导航栏（窄栏）。
 *
 * 侧栏折叠后只保留这一列图标：点击图标展开侧栏，并定位到对应面板。
 * 这样折叠不会丢失功能入口，只是把「面板」换成「图标」。
 */
import type { Component } from "vue";
import { PanelLeftOpen, PanelRightOpen } from "lucide-vue-next";

export interface RailItem {
  /** 唯一标识，用于展开后定位到对应面板 */
  key: string;
  /** 提示文字（悬停显示） */
  label: string;
  /** lucide 图标组件 */
  icon: Component;
  /** 是否高亮（表示当前正显示该面板） */
  active?: boolean;
}

const props = defineProps<{
  items: RailItem[];
  /** 所在侧：决定边框与悬停提示的方向 */
  side: "left" | "right";
}>();

const emit = defineEmits<{
  (e: "select", key: string): void;
  (e: "expand"): void;
}>();
</script>

<template>
  <div
    class="flex h-full w-14 shrink-0 flex-col items-center gap-1 py-3"
    :class="side === 'left' ? 'border-r border-gray-200 bg-white' : 'border-l border-gray-200 bg-white'"
  >
    <!-- 展开按钮：始终在顶部 -->
    <button
      type="button"
      class="mb-1 flex h-9 w-9 items-center justify-center rounded-xl text-slate-400 transition duration-200 hover:bg-gray-100 hover:text-slate-700"
      title="展开侧栏"
      @click="emit('expand')"
    >
      <component :is="side === 'left' ? PanelLeftOpen : PanelRightOpen" class="h-4 w-4" />
    </button>

    <div class="mb-1 h-px w-6 shrink-0 bg-gray-100" />

    <!-- 功能入口图标 -->
    <button
      v-for="item in props.items"
      :key="item.key"
      type="button"
      class="group relative flex h-10 w-10 shrink-0 items-center justify-center rounded-xl transition duration-200"
      :class="
        item.active
          ? 'bg-indigo-50 text-indigo-600'
          : 'text-slate-500 hover:bg-gray-100 hover:text-slate-900'
      "
      :title="item.label"
      @click="emit('select', item.key)"
    >
      <component :is="item.icon" class="h-[18px] w-[18px]" />
      <!-- 悬停显示名称，窄栏也能看懂 -->
      <span
        class="pointer-events-none absolute z-50 whitespace-nowrap rounded-lg bg-slate-900 px-2 py-1 text-[11px] text-white opacity-0 shadow-lg transition-opacity duration-150 group-hover:opacity-100"
        :class="side === 'left' ? 'left-full ml-2' : 'right-full mr-2'"
      >
        {{ item.label }}
      </span>
    </button>
  </div>
</template>
