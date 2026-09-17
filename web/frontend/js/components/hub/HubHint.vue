<template>
  <p class="text-stone-600 dark:text-stone-400">
    <GIcon name="box-open" class="mr-1" />
    {{ parts.before }}<a :href="url" target="_blank" class="text-info hover:text-info-hover">{{ host }}</a>{{ parts.after }}
  </p>
</template>

<script setup>
import {computed} from "vue"
import {GIcon} from "@gameap/ui"
import {hubHost, hubUrl} from "@/parts/hub"

const HOST_PLACEHOLDER = ':host'

const props = defineProps({
  text: {
    type: String,
    required: true,
  },
})

const url = hubUrl()
const host = hubHost()

// The translation is split around the placeholder instead of rendered with
// v-html, so it stays plain text and each language keeps its own word order.
const parts = computed(() => {
  const index = props.text.indexOf(HOST_PLACEHOLDER)
  if (index === -1) {
    return {before: `${props.text} `, after: ''}
  }

  return {
    before: props.text.slice(0, index),
    after: props.text.slice(index + HOST_PLACEHOLDER.length),
  }
})
</script>
