<template>
  <div v-if="accessBlocked || server.blocked" class="p-4 mb-4 text-warning-soft-text border border-orange-300 dark:border-orange-800 rounded-lg bg-warning-soft" role="alert">
    <div class="flex items-center">
      <GIcon name="warning" class="mr-1" />
      <h3 class="text-lg font-medium">{{ trans('servers.blocked_msg')}}</h3>
    </div>
  </div>

  <div v-else-if="server.installed === SERVER_NOT_INSTALLED" class="p-4 mb-4 text-warning-soft-text border border-orange-300 dark:border-orange-800 rounded-lg bg-warning-soft" role="alert">
    <div class="flex items-center">
      <GIcon name="warning" class="mr-1" />
      <h3 class="text-lg font-medium">{{ trans('servers.not_installed_msg')}}</h3>
    </div>
  </div>

  <div v-else-if="server.installed === SERVER_INSTALLING" class="p-4 mb-4 text-warning-soft-text border border-orange-300 dark:border-orange-800 rounded-lg bg-warning-soft" role="alert">
    <div class="flex items-center">
      <GIcon name="warning" class="mr-1" />
      <h3 class="text-lg font-medium">{{ trans('servers.installation_process_msg')}}</h3>
    </div>
  </div>

  <div v-else-if="!server.enabled" class="p-4 mb-4 text-warning-soft-text border border-orange-300 dark:border-orange-800 rounded-lg bg-warning-soft" role="alert">
    <div class="flex items-center">
      <GIcon name="warning" class="mr-1" />
      <h3 class="text-lg font-medium">{{ trans('servers.disabled_msg')}}</h3>
    </div>
  </div>

</template>

<script setup>
import {defineProps} from "vue"
import { GIcon } from "@gameap/ui"

const SERVER_NOT_INSTALLED = 0
const SERVER_INSTALLED = 1
const SERVER_INSTALLING = 2

const props = defineProps({
  server: null,
  accessBlocked: {
    type: Boolean,
    default: false,
  },
})
</script>