<template>
  <n-alert type="warning" :title="trans('servers.blocked_msg')" class="mb-3" data-testid="server-suspended-alert">
    <div v-for="line in details" :key="line" class="break-words">{{ line }}</div>
    <ServerControlButton
        v-if="serverId"
        command="unsuspend"
        :server-id="serverId"
        button="mt-2"
        button-color="green"
        button-size="small"
        icon="check"
        :text="trans('servers.unsuspend')"
    />
  </n-alert>
</template>

<script setup>
import {computed} from 'vue'
import {trans} from '@/i18n/i18n'
import {suspensionDetails} from '@/parts/suspension'
import ServerControlButton from '@/views/servertabs/ServerControlButton.vue'

const props = defineProps({
  suspension: {
    type: Object,
    default: null,
  },
  // Given for an administrator, who can lift the suspension right here.
  serverId: {
    type: Number,
    default: null,
  },
})

const details = computed(() => suspensionDetails(props.suspension))
</script>
