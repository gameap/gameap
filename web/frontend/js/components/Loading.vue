<template>
  <div class="flex justify-center">
    <TransitionRoot
        :show="showTransition"
        enter="transition-opacity duration-[2000ms]"
        enter-from="opacity-0"
        enter-to="opacity-100"
        leave="transition-opacity duration-150"
        leave-from="opacity-100"
        leave-to="opacity-0"
    >
      <div class="fa-3x">
        <i class="fa-solid fa-gear fa-spin"></i>
      </div>
    </TransitionRoot>
  </div>
</template>

<script setup>
import { onMounted, ref } from "vue"
import { TransitionRoot } from '@headlessui/vue'

const showTransition = ref(false)

onMounted(() => {
  setTimeout(() => {
    showTransition.value = true
  }, 10)
})


</script>