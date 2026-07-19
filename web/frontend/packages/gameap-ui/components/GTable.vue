<template>
  <n-table
    v-bind="mergedProps"
  >
    <slot />
  </n-table>
</template>

<script setup>
import { computed, useAttrs } from 'vue'
import { NTable } from 'naive-ui'

const props = defineProps({
  bordered: {
    type: Boolean,
    default: false
  },
  singleLine: {
    type: Boolean,
    default: true
  },
  size: {
    type: String,
    default: undefined
  }
})

defineOptions({
  inheritAttrs: false
})

const attrs = useAttrs()

const mergedProps = computed(() => ({
  bordered: props.bordered,
  singleLine: props.singleLine,
  ...(props.size && { size: props.size }),
  ...attrs
}))
</script>
