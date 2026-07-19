<template>
  <n-input
    v-bind="mergedProps"
  >
    <template v-for="(_, slotName) in $slots" :key="slotName" #[slotName]="slotProps">
      <slot :name="slotName" v-bind="slotProps || {}" />
    </template>
  </n-input>
</template>

<script setup>
import { computed, useAttrs } from 'vue'
import { NInput } from 'naive-ui'

const props = defineProps({
  value: {
    type: [String, Array],
    default: undefined
  },
  type: {
    type: String,
    default: 'text'
  },
  placeholder: {
    type: String,
    default: ''
  },
  disabled: {
    type: Boolean,
    default: false
  },
  readonly: {
    type: Boolean,
    default: false
  },
  clearable: {
    type: Boolean,
    default: false
  },
  size: {
    type: String,
    default: undefined
  }
})

defineOptions({
  inheritAttrs: false
})

const emit = defineEmits(['update:value'])

const attrs = useAttrs()

const mergedProps = computed(() => ({
  value: props.value,
  'onUpdate:value': (val) => emit('update:value', val),
  type: props.type,
  placeholder: props.placeholder,
  disabled: props.disabled,
  readonly: props.readonly,
  clearable: props.clearable,
  ...(props.size && { size: props.size }),
  ...attrs
}))
</script>
