<template>
  <span>
    <template v-for="(part, index) in parts" :key="index">
      <a v-if="index > 0" :href="url" target="_blank" class="text-info hover:text-info-hover">{{ host }}</a>{{ part }}
    </template>
  </span>
</template>

<script setup>
import {computed} from "vue"
import {hubHost, hubUrl} from "@/parts/hub"

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
const parts = computed(() => props.text.split(':host'))
</script>
