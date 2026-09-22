<template>
  <n-modal
      :show="show"
      :on-update:show="(v) => $emit('update:show', v)"
      :auto-focus="false"
      preset="card"
      style="width: min(96vw, 1300px); max-width: 1300px;"
      :bordered="false"
      :segmented="{ content: 'soft', footer: 'soft' }"
      :close-on-esc="true"
      :mask-closable="true"
  >
    <template #header>
      <div class="flex items-center gap-3 flex-wrap">
        <GIcon name="metrics" class="text-xl" />
        <span class="font-semibold">{{ serverName || ('#' + serverId) }}</span>
        <GStatusBadge
            :status="online ? 'success' : 'error'"
            :text="online ? trans('servers.active') : trans('servers.inactive')"
        />
      </div>
    </template>

    <div class="overflow-y-auto pr-1 max-h-[75vh]">
      <ServerStatistics v-if="show" :server-id="serverId" :cpu-limit-percent="cpuLimitPercent" />
    </div>
  </n-modal>
</template>

<script setup>
import { defineAsyncComponent } from 'vue'
import { NModal } from 'naive-ui'
import { GIcon, GStatusBadge } from '@gameap/ui'
import { trans } from '@/i18n/i18n'

const ServerStatistics = defineAsyncComponent(() =>
    import('./ServerStatistics.vue' /* webpackChunkName: "components/server" */),
)

defineProps({
    show: { type: Boolean, default: false },
    serverId: { type: Number, default: null },
    serverName: { type: String, default: '' },
    online: { type: Boolean, default: false },
    cpuLimitPercent: { type: Number, default: null },
})

defineEmits(['update:show'])
</script>
