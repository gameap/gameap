<template>
  <n-empty
    v-bind="mergedProps"
  >
    <template v-for="(_, slotName) in $slots" :key="slotName" #[slotName]="slotProps">
      <slot :name="slotName" v-bind="slotProps || {}" />
    </template>
  </n-empty>
</template>

<script setup>
import { computed, useAttrs } from 'vue'
import { NEmpty } from 'naive-ui'

const props = defineProps({
  description: {
    type: String,
    default: undefined
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
  ...(props.description && { description: props.description }),
  ...(props.size && { size: props.size }),
  ...attrs
}))
</script>
