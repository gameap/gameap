<template>
  <span :class="spanClass">{{ statusText }}</span>
</template>

<script setup>
import {computed, defineProps} from "vue"

const props = defineProps({
  status: {
    type: String,
    default: '',
  },
  text: {
    type: String,
  },
  color: {
    type: String,
    default: '',
  },
})

const badgeClasses = {
  waiting: 'badge-light',
  working: 'badge-blue',
  error: 'badge-red',
  success: 'badge-green',
  canceled: 'badge-stone',
}

const colorClasses = {
  light: 'badge-light',
  blue: 'badge-blue',
  red: 'badge-red',
  green: 'badge-green',
  orange: 'badge-orange',
  stone: 'badge-stone',
}

// Status badges share a minimum width so they line up in table columns;
// color badges are tags and hug their content.
const spanClass = computed(() => {
  if (props.color) {
    const base = colorClasses[props.color] ?? 'badge-light'

    return `${base} inline-flex items-center justify-center whitespace-nowrap`
  }

  const base = badgeClasses[props.status] ?? 'badge-light'

  return `${base} inline-flex items-center justify-center min-w-[5.5rem]`
})

const statusText = computed(() => {
  return props.text ? props.text : props.status
})
</script>
