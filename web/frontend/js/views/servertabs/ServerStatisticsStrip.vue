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

    <!-- One row needs ~1000px of strip, which the page leaves only from xl on;
         narrower screens get two cells per row, phones one. -->
    <div
        class="grid grid-cols-1 md:grid-cols-2 gap-x-8 gap-y-3
               xl:flex xl:items-center xl:gap-0 xl:divide-x divide-stone-200 dark:divide-stone-700"
    >
      <div class="flex items-center gap-2 xl:flex-1 xl:px-4" data-testid="server-stats-cpu">
        <span class="shrink-0 text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400 w-10">CPU</span>
        <n-progress
            v-if="hasCpuBar"
            type="line"
            :percentage="cpuWidth"
            :color="cpuColor"
            :height="10"
            :border-radius="2"
            :show-indicator="false"
            class="flex-1"
        />
        <span
            class="text-xs font-mono tabular-nums text-right whitespace-nowrap"
            :class="hasCpuBar ? 'min-w-16' : 'flex-1'"
        >{{ formatPercent(cpuPercent) }}<span
            v-if="cpuLimitPercent !== null"
            class="text-stone-400 dark:text-stone-500"
        > / {{ formatLimit(cpuLimitPercent) }}</span></span>
      </div>

      <div class="flex items-center gap-2 xl:flex-1 xl:px-4" :title="memTitle">
        <span class="shrink-0 text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400 w-10">MEM</span>
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
        <span v-else class="flex-1 text-xs font-mono tabular-nums text-right whitespace-nowrap">{{ formatBytes(memBytes) }}</span>
      </div>

      <!-- The card sets word-break: break-word, so a squeezed cell would split
           words; each direction stays whole and the pair wraps between them. -->
      <div class="flex items-center justify-between gap-2 xl:flex-1 xl:px-4">
        <span class="shrink-0 text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400">NET</span>
        <span class="flex flex-wrap justify-end gap-x-2 text-xs font-mono tabular-nums">
          <span class="whitespace-nowrap"><span class="text-chart-7">↑</span> {{ formatBitrate(netIn) }}</span>
          <span class="whitespace-nowrap"><span class="text-chart-3">↓</span> {{ formatBitrate(netOut) }}</span>
        </span>
      </div>

      <div class="flex items-center justify-between gap-2 xl:flex-1 xl:px-4">
        <span class="shrink-0 text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400">DISK</span>
        <span class="flex flex-wrap justify-end gap-x-2 text-xs font-mono tabular-nums">
          <span class="whitespace-nowrap"><span class="text-stone-400">R</span> {{ formatBitrate(diskRead) }}</span>
          <span class="whitespace-nowrap"><span class="text-stone-400">W</span> {{ formatBitrate(diskWrite) }}</span>
        </span>
      </div>

      <div class="hidden xl:flex items-center justify-center xl:pl-4 text-stone-400 dark:text-stone-500">
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
    cpuLimitPercent: { type: Number, default: null },
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

// CPU is a percentage of one core, so only a limit gives the bar a full scale:
// without one, 250% on a multi-core host is not a saturated server.
const hasCpuBar = computed(() => Number.isFinite(props.cpuLimitPercent) && props.cpuLimitPercent > 0)

const cpuOfLimit = computed(() => {
    if (!hasCpuBar.value || cpuPercent.value === null || cpuPercent.value === undefined) return null

    return cpuPercent.value / props.cpuLimitPercent * 100
})

const cpuWidth = computed(() => clamp(cpuOfLimit.value, 0, 100))
const memWidth = computed(() => clamp(memPercent.value, 0, 100))

const cpuColor = computed(() => paletteFor(cpuOfLimit.value))
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

function formatLimit(v) {
    return Number.isFinite(v) ? `${Number(v.toFixed(1))}%` : '∞'
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
