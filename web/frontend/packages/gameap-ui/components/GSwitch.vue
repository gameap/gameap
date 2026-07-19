<template>
  <n-switch
    v-bind="mergedProps"
  >
    <template v-for="(_, slotName) in $slots" :key="slotName" #[slotName]="slotProps">
      <slot :name="slotName" v-bind="slotProps || {}" />
    </template>
  </n-switch>
</template>

<script setup>
import { computed, useAttrs } from 'vue'
import { NSwitch } from 'naive-ui'

const props = defineProps({
  value: {
    type: Boolean,
    default: false
  },
  disabled: {
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
  disabled: props.disabled,
  ...(props.size && { size: props.size }),
  ...attrs
}))
</script>
