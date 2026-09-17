<template>
  <p class="text-stone-600 dark:text-stone-400">
    <GIcon name="box-open" class="mr-2" />
    <HubText :text="text" />
  </p>
</template>

<script setup>
import {GIcon} from "@gameap/ui"
import HubText from "./HubText.vue"

defineProps({
  text: {
    type: String,
    required: true,
  },
})
</script>
