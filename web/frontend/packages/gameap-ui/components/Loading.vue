<template>
  <div class="flex justify-center">
    <Transition
        enter-active-class="transition-opacity duration-150"
        enter-from-class="opacity-0"
        enter-to-class="opacity-100"
        leave-active-class="transition-opacity duration-150"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
    >
      <div v-if="showTransition" class="fa-3x">
        <GIcon name="loading" />
      </div>
    </Transition>
  </div>
</template>

<script setup>
import { onMounted, onUnmounted, ref } from "vue"
import GIcon from './GIcon.vue'

// A response that arrives within the delay never gets a spinner, so a fast page
// does not blink. Once the delay is over the spinner has to be readable right
// away, hence the short fade.
const SHOW_DELAY = 150

const showTransition = ref(false)
let showTimeout = null

onMounted(() => {
  showTimeout = setTimeout(() => {
    showTransition.value = true
  }, SHOW_DELAY)
})

onUnmounted(() => {
  clearTimeout(showTimeout)
})
</script>
