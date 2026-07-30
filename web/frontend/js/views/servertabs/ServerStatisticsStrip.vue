<template>
  <n-card
      class="server-stats-strip cursor-pointer"
      size="small"
      :bordered="true"
      :segmented="{ content: true }"
      role="button"
      tabindex="0"
      :aria-label="trans('servers.statistics_open')"
      @click="$emit('open')"
      @keydown.enter="$emit('open')"
      @keydown.space.prevent="$emit('open')"
  >
    <div v-if="errorMessage" class="text-xs text-orange-600 dark:text-orange-500 mb-2">
      {{ errorMessage }}
    </div>

    <div
        class="flex flex-col md:flex-row md:items-center gap-3 md:gap-0
               md:divide-x divide-stone-200 dark:divide-stone-700"
    >
      <div class="flex items-center gap-2 md:flex-1 md:px-4">
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
        <span class="text-xs font-mono tabular-nums w-16 text-right">{{ formatPercent(cpuPercent) }}</span>
      </div>

      <div class="flex items-center gap-2 md:flex-1 md:px-4" :title="memTitle">
        <span class="text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400 w-10">MEM</span>
        <template v-if="hasMemBar">
          <n-progress
              type="line"
              :percentage="memWidth"
              :color="memColor"
              :height="10"
              :border-radius="2"
              :show-indicator="false"
              class="flex-1"
          />
          <span class="text-xs font-mono tabular-nums w-16 text-right">{{ formatPercent(memPercent) }}</span>
        </template>
        <span v-else class="flex-1 text-xs font-mono tabular-nums text-right">{{ formatBytes(memBytes) }}</span>
      </div>

      <div class="flex items-center justify-between gap-2 md:flex-1 md:px-4">
        <span class="text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400">NET</span>
        <span class="text-xs font-mono tabular-nums">
          <span class="text-chart-7">↑</span> {{ formatBitrate(netIn) }}
          <span class="text-chart-3 ml-2">↓</span> {{ formatBitrate(netOut) }}
        </span>
      </div>

      <div class="flex items-center justify-between gap-2 md:flex-1 md:px-4">
        <span class="text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400">DISK</span>
        <span class="text-xs font-mono tabular-nums">
          <span class="text-stone-400">R</span> {{ formatBitrate(diskRead) }}
          <span class="text-stone-400 ml-2">W</span> {{ formatBitrate(diskWrite) }}
        </span>
      </div>

      <div class="hidden md:flex items-center justify-center md:pl-4 text-stone-400 dark:text-stone-500">
        <GIcon name="metrics" class="text-lg" />
      </div>
    </div>
  </n-card>
</template>

<script setup>
import { computed } from 'vue'
import { NCard, NProgress } from 'naive-ui'
import { GIcon } from '@gameap/ui'
import { useServerMetricsWebSocket } from '@/composables/useServerMetricsWebSocket'
import { useThemeVars } from '@/utils/theme'
import { trans } from '@/i18n/i18n'

const props = defineProps({
    serverId: { type: Number, required: true },
})

defineEmits(['open'])

const { chartPalette, statusColors } = useThemeVars()

const {
    errorMessage,
    cpuSeries,
    memoryPercentSeries,
    memoryBytesSeries,
    diskReadSeries,
    diskWriteSeries,
    networkInSeries,
    networkOutSeries,
} = useServerMetricsWebSocket(() => props.serverId)

function lastVal(list) {
    for (const s of list) {
        if (s.points && s.points.length) {
            return s.points[s.points.length - 1].v
        }
    }

    return null
}

const cpuPercent = computed(() => lastVal(cpuSeries.value))
const memPercent = computed(() => lastVal(memoryPercentSeries.value))
const memBytes = computed(() => lastVal(memoryBytesSeries.value))
const netIn = computed(() => lastVal(networkInSeries.value))
const netOut = computed(() => lastVal(networkOutSeries.value))
const diskRead = computed(() => lastVal(diskReadSeries.value))
const diskWrite = computed(() => lastVal(diskWriteSeries.value))

const hasMemBar = computed(() => memPercent.value !== null && memPercent.value !== undefined)

const cpuWidth = computed(() => clamp(cpuPercent.value, 0, 100))
const memWidth = computed(() => clamp(memPercent.value, 0, 100))

const cpuColor = computed(() => paletteFor(cpuPercent.value))
const memColor = computed(() => paletteFor(memPercent.value))

const memTitle = computed(() => {
    const parts = []
    if (memBytes.value !== null) parts.push(formatBytes(memBytes.value))
    if (memPercent.value !== null) parts.push(formatPercent(memPercent.value))

    return parts.join(' · ')
})

function clamp(v, min, max) {
    if (v === null || v === undefined || Number.isNaN(v)) return 0

    return Math.max(min, Math.min(max, Number(v)))
}

function paletteFor(v) {
    if (v === null || v === undefined) return chartPalette.value[6]
    if (v > 90) return statusColors.value.danger
    if (v > 75) return statusColors.value.warning

    return chartPalette.value[6]
}

function formatPercent(v) {
    if (v === null || v === undefined || Number.isNaN(v)) return '—'

    return `${Number(v).toFixed(1)}%`
}

function formatBytes(v) {
    if (v === null || v === undefined || Number.isNaN(v)) return '—'
    const u = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
    let i = 0
    let n = Number(v)
    while (n >= 1024 && i < u.length - 1) {
        n /= 1024
        i++
    }

    return `${n.toFixed(1)} ${u[i]}`
}

function formatBitrate(v) {
    if (v === null || v === undefined || Number.isNaN(v)) return '—'

    return `${formatBytes(v)}/s`
}
</script>
