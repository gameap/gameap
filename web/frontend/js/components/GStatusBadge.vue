<template>
  <span :class="spanClass">{{ statusText }}</span>
</template>

<script setup>
import {computed, defineProps} from "vue"

const props = defineProps({
  status: {
    type: String,
    required: true
  },
  text: {
    type: String,
  },
})

const badgeClasses = {
  waiting: 'badge-light',
  working: 'badge-blue',
  error: 'badge-red',
  success: 'badge-green',
  canceled: 'badge-stone',
}

const spanClass = computed(() => {
  return badgeClasses[props.status] ?? 'badge-light'
})

const statusText = computed(() => {
  return props.text ? props.text : props.status
})
</script>