<script setup lang="ts">
import { nextTick, watch } from "vue";
import { ImagePlus, Send, Trash2 } from "lucide-vue-next";

const props = defineProps<{
  messages: Array<{ role: "user" | "ai"; content: string }>;
  chatInput: string;
  isChatting: boolean;
  hasConversation: boolean;
  renderMarkdown: (text: string) => string;
}>();

const emit = defineEmits<{
  "update:chatInput": [value: string];
  send: [];
  clear: [];
  uploadImage: [event: Event];
}>();

function scrollBottom() {
  nextTick(() => {
    const el = document.getElementById("workspace-chat-messages");
    if (el) el.scrollTop = el.scrollHeight;
  });
}

watch(() => props.messages.length, scrollBottom);
watch(() => props.isChatting, scrollBottom);
</script>

<template>
  <section class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-[18px] border border-gray-200 bg-white shadow-sm">
    <div class="flex shrink-0 items-center justify-between border-b border-gray-200 px-4 py-3">
      <div class="min-w-0">
        <h2 class="text-base font-semibold text-slate-900">AI 学习导师</h2>
        <p class="mt-0.5 truncate text-[13px] text-slate-500">基于笔记与图谱上下文回答</p>
      </div>
      <button
        v-if="hasConversation"
        type="button"
        class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-xl px-3 text-[13px] font-medium text-slate-500 transition duration-200 hover:bg-gray-100 hover:text-red-500"
        @click="emit('clear')"
      >
        <Trash2 class="h-3.5 w-3.5" />
        清空
      </button>
    </div>

    <div id="workspace-chat-messages" class="min-h-0 flex-1 space-y-3 overflow-y-auto px-4 py-3">
      <div v-if="messages.length === 0" class="flex h-full min-h-[80px] flex-col items-center justify-center text-center">
        <p class="text-sm font-medium text-slate-700">还没有对话</p>
        <p class="mt-1 max-w-[200px] text-[13px] text-slate-500">生成图谱后可在此提问</p>
      </div>

      <div v-for="(m, i) in messages" :key="i" :class="m.role === 'user' ? 'flex justify-end' : 'flex justify-start'">
        <div
          v-if="m.role === 'user'"
          class="max-w-[90%] rounded-2xl bg-indigo-600 px-4 py-2.5 text-sm leading-relaxed text-white"
        >
          {{ m.content }}
        </div>
        <div
          v-else
          class="markdown-body max-w-[95%] rounded-2xl bg-slate-100 px-4 py-2.5 text-sm leading-relaxed text-slate-700"
          v-html="renderMarkdown(m.content)"
        />
      </div>

      <div v-if="isChatting" class="text-[13px] text-slate-400">AI 正在思考…</div>
    </div>

    <div class="flex shrink-0 items-center gap-2 border-t border-gray-200 p-3">
      <label
        class="inline-flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center rounded-xl border border-gray-200 text-slate-500 transition duration-200 hover:bg-gray-100"
        title="上传图片"
      >
        <ImagePlus class="h-4 w-4" />
        <input class="hidden" type="file" accept="image/*" @change="emit('uploadImage', $event)" />
      </label>
      <input
        :value="chatInput"
        :disabled="isChatting"
        class="h-11 min-w-0 flex-1 rounded-xl border border-gray-200 px-3 text-sm outline-none transition duration-200 placeholder:text-slate-400 focus:border-indigo-500 disabled:opacity-60"
        placeholder="输入问题…"
        @input="emit('update:chatInput', ($event.target as HTMLInputElement).value)"
        @keyup.enter="emit('send')"
      />
      <button
        type="button"
        :disabled="(!chatInput.trim() && !isChatting) || isChatting"
        class="inline-flex h-11 shrink-0 items-center gap-1.5 rounded-xl bg-indigo-600 px-3 text-sm font-medium text-white transition duration-200 hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50"
        @click="emit('send')"
      >
        <Send class="h-4 w-4" />
        发送
      </button>
    </div>
  </section>
</template>
