<template>
  <n-card
    v-bind="mergedProps"
  >
    <template v-for="(_, slotName) in $slots" :key="slotName" #[slotName]="slotProps">
      <slot :name="slotName" v-bind="slotProps || {}" />
    </template>
  </n-card>
</template>

<script setup>
import { computed, useAttrs } from 'vue'
import { NCard } from 'naive-ui'

const props = defineProps({
  title: {
    type: String,
    default: ''
  },
  size: {
    type: String,
    default: 'small'
  },
  bordered: {
    type: Boolean,
    default: true
  },
  segmented: {
    type: [Boolean, Object],
    default: () => ({ content: true, footer: 'soft' })
  },
  headerClass: {
    type: String,
    default: 'g-card-header'
  }
})

defineOptions({
  inheritAttrs: false
})

const attrs = useAttrs()

const mergedProps = computed(() => ({
  title: props.title,
  size: props.size,
  bordered: props.bordered,
  segmented: props.segmented,
  headerClass: props.headerClass,
  ...attrs
}))
</script>
