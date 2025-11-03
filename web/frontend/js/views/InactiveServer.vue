<template>
  <div v-if="server.installed === SERVER_NOT_INSTALLED" id="alert-additional-content-4" class="p-4 mb-4 text-yellow-800 border border-yellow-300 rounded-lg bg-yellow-50 dark:bg-stone-800 dark:text-yellow-300 dark:border-yellow-800" role="alert">
    <div class="flex items-center">
      <i class="fa-solid fa-triangle-exclamation mr-1"></i>
      <h3 class="text-lg font-medium">{{ trans('servers.not_installed_msg')}}</h3>
    </div>
  </div>

  <div v-else-if="server.installed === SERVER_INSTALLING" id="alert-additional-content-4" class="p-4 mb-4 text-yellow-800 border border-yellow-300 rounded-lg bg-yellow-50 dark:bg-stone-800 dark:text-yellow-300 dark:border-yellow-800" role="alert">
    <div class="flex items-center">
      <i class="fa-solid fa-triangle-exclamation mr-1"></i>
      <h3 class="text-lg font-medium">{{ trans('servers.installation_process_msg')}}</h3>
    </div>
  </div>

  <div v-else-if="server.blocked" id="alert-additional-content-4" class="p-4 mb-4 text-yellow-800 border border-yellow-300 rounded-lg bg-yellow-50 dark:bg-stone-800 dark:text-yellow-300 dark:border-yellow-800" role="alert">
    <div class="flex items-center">
      <i class="fa-solid fa-triangle-exclamation mr-1"></i>
      <h3 class="text-lg font-medium">{{ trans('servers.blocked_msg')}}</h3>
    </div>
  </div>

  <div v-else-if="!server.enabled" id="alert-additional-content-4" class="p-4 mb-4 text-yellow-800 border border-yellow-300 rounded-lg bg-yellow-50 dark:bg-stone-800 dark:text-yellow-300 dark:border-yellow-800" role="alert">
    <div class="flex items-center">
      <i class="fa-solid fa-triangle-exclamation mr-1"></i>
      <h3 class="text-lg font-medium">{{ trans('servers.disabled_msg')}}</h3>
    </div>
  </div>

</template>

<script setup>
import {defineProps} from "vue"

const SERVER_NOT_INSTALLED = 0
const SERVER_INSTALLED = 1
const SERVER_INSTALLING = 2

const props = defineProps({
  server: null
})
</script>