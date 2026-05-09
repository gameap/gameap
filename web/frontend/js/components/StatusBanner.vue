<template>
  <n-alert
      :type="type"
      :title="title"
      :show-icon="true"
      :bordered="false"
      class="mb-3"
  >
    <template v-if="icon" #icon>
      <GIcon :name="icon" />
    </template>

    <div class="flex items-center justify-between gap-3">
      <div class="min-w-0">
        <div v-if="text" class="text-sm leading-snug">{{ text }}</div>
        <slot></slot>
      </div>
      <div v-if="$slots.actions" class="shrink-0 flex items-center gap-2">
        <slot name="actions"></slot>
      </div>
    </div>
  </n-alert>
</template>

<script setup>
import { NAlert } from 'naive-ui'

defineProps({
    type: {
        type: String,
        default: 'info',
        validator: (value) => ['default', 'info', 'success', 'warning', 'error'].includes(value),
    },
    title: {
        type: String,
        default: '',
    },
    text: {
        type: String,
        default: '',
    },
    icon: {
        type: String,
        default: '',
    },
})
</script>
