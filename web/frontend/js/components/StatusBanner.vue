<template>
  <div
      :class="bannerClasses"
      class="fixed top-16 inset-x-0 z-40 px-4 py-2 shadow-md transition-colors"
      role="status"
      aria-live="polite"
  >
    <div class="flex items-center justify-between gap-3 max-w-screen-2xl mx-auto">
      <div class="flex items-center gap-3 min-w-0">
        <GIcon v-if="resolvedIcon" :name="resolvedIcon" class="shrink-0" />
        <div class="min-w-0">
          <div v-if="title" class="font-semibold leading-tight">{{ title }}</div>
          <div v-if="text" class="text-sm leading-snug truncate sm:whitespace-normal">{{ text }}</div>
          <slot></slot>
        </div>
      </div>
      <div v-if="$slots.actions" class="shrink-0 flex items-center gap-2">
        <slot name="actions"></slot>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
    type: {
        type: String,
        default: 'info',
        validator: (value) => ['info', 'success', 'warning', 'error'].includes(value),
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

const typeStyles = {
    info: 'bg-blue-500 text-white dark:bg-blue-700',
    success: 'bg-emerald-500 text-white dark:bg-emerald-700',
    warning: 'bg-amber-500 text-white dark:bg-amber-700 dark:text-stone-100',
    error: 'bg-red-600 text-white dark:bg-red-700',
}

const defaultIcons = {
    info: 'info',
    success: 'check',
    warning: 'warning',
    error: 'offline',
}

const bannerClasses = computed(() => typeStyles[props.type])
const resolvedIcon = computed(() => props.icon || defaultIcons[props.type])
</script>
