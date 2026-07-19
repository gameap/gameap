<template>
  <n-data-table
    v-bind="mergedProps"
  >
    <template v-for="(_, slotName) in $slots" :key="slotName" #[slotName]="slotProps">
      <slot :name="slotName" v-bind="slotProps || {}" />
    </template>
  </n-data-table>
</template>

<script setup>
import { computed, useAttrs } from 'vue'
import { NDataTable } from 'naive-ui'

const props = defineProps({
  bordered: {
    type: Boolean,
    default: false
  },
  singleLine: {
    type: Boolean,
    default: true
  },
  columns: {
    type: Array,
    default: () => []
  },
  data: {
    type: Array,
    default: () => []
  },
  loading: {
    type: Boolean,
    default: false
  },
  pagination: {
    type: [Object, Boolean],
    default: false
  },
  remote: {
    type: Boolean,
    default: false
  }
})

defineOptions({
  inheritAttrs: false
})

const attrs = useAttrs()

const mergedProps = computed(() => ({
  bordered: props.bordered,
  singleLine: props.singleLine,
  columns: props.columns,
  data: props.data,
  loading: props.loading,
  pagination: props.pagination,
  remote: props.remote,
  ...attrs
}))
</script>
