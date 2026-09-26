<template>
  <ServerSuspendedAlert
      v-if="accessBlocked || server.blocked"
      :suspension="suspension"
      :server-id="serverId"
  />

  <n-alert v-else-if="server.installed === SERVER_NOT_INSTALLED" type="warning" :title="trans('servers.not_installed_msg')" class="mb-4" />

  <n-alert v-else-if="server.installed === SERVER_INSTALLING" type="warning" :title="trans('servers.installation_process_msg')" class="mb-4" />

  <n-alert v-else-if="!server.enabled" type="warning" :title="trans('servers.disabled_msg')" class="mb-4" />
</template>

<script setup>
import ServerSuspendedAlert from "@/components/servers/ServerSuspendedAlert.vue"

const SERVER_NOT_INSTALLED = 0
const SERVER_INSTALLING = 2

defineProps({
  server: null,
  accessBlocked: {
    type: Boolean,
    default: false,
  },
  suspension: {
    type: Object,
    default: null,
  },
  // Given for an administrator, who can lift a suspension from here.
  serverId: {
    type: Number,
    default: null,
  },
})
</script>
