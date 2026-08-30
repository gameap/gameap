<template>
  <n-card
      class="node-card cursor-pointer"
      :class="{ 'opacity-60 hover:opacity-90': !online }"
      size="small"
      :bordered="true"
      :segmented="{ content: true }"
      role="button"
      tabindex="0"
      @click="$emit('open-details', node.id)"
      @keydown.enter="$emit('open-details', node.id)"
      @keydown.space.prevent="$emit('open-details', node.id)"
  >
    <template #header>
      <div class="flex items-center gap-2 min-w-0">
        <GIcon :name="osIconName" class="text-lg flex-none" />
        <span class="font-semibold truncate">{{ node.name }}</span>
      </div>
    </template>

    <template #header-extra>
      <GStatusBadge
          :status="online ? 'success' : 'error'"
          :text="online ? trans('dedicated_servers.online') : trans('dedicated_servers.offline')"
      />
    </template>

    <div class="text-sm text-stone-500 dark:text-stone-400 mb-3 flex flex-wrap gap-x-3 gap-y-1">
      <span v-if="node.location">{{ node.location }}</span>
      <span v-if="node.provider">· {{ node.provider }}</span>
      <span v-if="primaryIp" class="font-mono">· {{ primaryIp }}</span>
      <span v-if="daemonVersion" class="inline-flex items-center gap-x-1 whitespace-nowrap">
        <span v-if="node.location || node.provider || primaryIp">·</span>
        <span class="font-mono tabular-nums">v{{ daemonVersion }}</span>
        <template v-if="outdated && latestVersion">
          <span class="text-stone-400" aria-hidden="true">&rarr;</span>
          <a
              class="text-orange-500 font-medium hover:underline"
              :href="latestUrl"
              target="_blank"
              @click.stop
          >{{ latestVersion }}</a>
          <NTooltip trigger="hover">
            <template #trigger>
              <GIcon name="warning" class="text-orange-500" />
            </template>
            {{ trans('dedicated_servers.daemon_update_available') }}
          </NTooltip>
        </template>
      </span>
    </div>

    <template v-if="online && hasMetrics">
      <div class="flex items-center gap-2 mb-2">
        <span class="text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400 w-10">CPU</span>
        <n-progress
            type="line"
            :percentage="cpuWidth"
            :color="cpuColor"
            :height="10"
            :border-radius="2"
            :show-indicator="false"
            class="flex-1"
        />
        <span class="text-xs font-mono tabular-nums w-14 text-right">{{ formatPercent(cpuPercent) }}</span>
      </div>

      <div class="flex items-center gap-2 mb-3">
        <span class="text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400 w-10">MEM</span>
        <n-progress
            type="line"
            :percentage="memWidth"
            :color="memColor"
            :height="10"
            :border-radius="2"
            :show-indicator="false"
            class="flex-1"
        />
        <span class="text-xs font-mono tabular-nums w-14 text-right">{{ formatPercent(memPercent) }}</span>
      </div>

      <div v-if="hasNet" class="flex justify-between text-xs font-mono text-stone-500 dark:text-stone-400">
        <span>
          <span class="text-chart-7">↑</span>
          <span class="ml-1 tabular-nums">{{ formatBitrate(snapshot?.netInBps) }}</span>
        </span>
        <span>
          <span class="text-chart-3">↓</span>
          <span class="ml-1 tabular-nums">{{ formatBitrate(snapshot?.netOutBps) }}</span>
        </span>
      </div>
    </template>

    <div
        v-else
        class="text-center text-xs text-stone-400 dark:text-stone-500 italic py-2"
    >
      {{ online ? trans('dedicated_servers.no_metrics_data') : '— — —' }}
    </div>
  </n-card>
</template>

<script setup>
import { computed } from 'vue'
import { NCard, NProgress, NTooltip } from 'naive-ui'
import { GIcon, GStatusBadge } from '@gameap/ui'
import { useThemeVars } from '@/utils/theme'
import { trans } from '@/i18n/i18n'

const props = defineProps({
    node: { type: Object, required: true },
    online: { type: Boolean, default: false },
    snapshot: { type: Object, default: null },
    daemonVersion: { type: String, default: '' },
    outdated: { type: Boolean, default: false },
    latestVersion: { type: String, default: '' },
    latestUrl: { type: String, default: '' },
})

defineEmits(['open-details'])

const osIconName = computed(() => {
    const os = String(props.node?.os || '').toLowerCase()
    if (os.startsWith('w')) return 'windows'
    if (os.startsWith('m')) return 'apple'
    return 'linux'
})

const primaryIp = computed(() => {
    const ip = props.node?.ip
    if (!Array.isArray(ip) || ip.length === 0) return ''
    return ip[0]
})

const cpuPercent = computed(() => props.snapshot?.cpuPercent ?? null)
const memPercent = computed(() => props.snapshot?.memPercent ?? null)

const cpuWidth = computed(() => clamp(cpuPercent.value, 0, 100))
const memWidth = computed(() => clamp(memPercent.value, 0, 100))

const { chartPalette, statusColors } = useThemeVars()

const cpuColor = computed(() => paletteFor(cpuPercent.value, chartPalette.value[1]))
const memColor = computed(() => paletteFor(memPercent.value, chartPalette.value[6]))

const hasMetrics = computed(() =>
    cpuPercent.value !== null
    || memPercent.value !== null
    || props.snapshot?.netInBps != null
    || props.snapshot?.netOutBps != null,
)

const hasNet = computed(() =>
    props.snapshot?.netInBps != null
    || props.snapshot?.netOutBps != null,
)

function clamp(v, min, max) {
    if (v === null || v === undefined || Number.isNaN(v)) return 0
    return Math.max(min, Math.min(max, Number(v)))
}

function paletteFor(v, base) {
    if (v === null || v === undefined) return base
    if (v > 90) return statusColors.value.danger
    if (v > 75) return statusColors.value.warning
    return base
}

function formatPercent(v) {
    if (v === null || v === undefined || Number.isNaN(v)) return '—'
    return `${Number(v).toFixed(1)}%`
}

function formatBitrate(bps) {
    if (bps === null || bps === undefined || Number.isNaN(bps)) return '—'
    const u = ['B/s', 'KiB/s', 'MiB/s', 'GiB/s']
    let i = 0
    let n = Number(bps)
    while (n >= 1024 && i < u.length - 1) {
        n /= 1024
        i++
    }
    return `${n.toFixed(1)} ${u[i]}`
}
</script>
