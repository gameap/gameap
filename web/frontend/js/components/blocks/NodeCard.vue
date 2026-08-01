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
      <n-tag
          v-if="online"
          type="success"
          size="small"
          round
          :bordered="false"
      >
        <template #icon>
          <span class="online-dot" />
        </template>
        {{ trans('dedicated_servers.online') }}
      </n-tag>
      <n-tag
          v-else
          type="error"
          size="small"
          round
          :bordered="false"
      >
        {{ trans('dedicated_servers.offline') }}
      </n-tag>
    </template>

    <div class="text-sm text-stone-500 dark:text-stone-400 mb-3 flex flex-wrap gap-x-3 gap-y-1">
      <span v-if="node.location">{{ node.location }}</span>
      <span v-if="node.provider">· {{ node.provider }}</span>
      <span v-if="primaryIp" class="font-mono">· {{ primaryIp }}</span>
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
import { NCard, NProgress, NTag } from 'naive-ui'
import { GIcon } from '@gameap/ui'
import { useThemeVars } from '@/utils/theme'
import { trans } from '@/i18n/i18n'

const props = defineProps({
    node: { type: Object, required: true },
    online: { type: Boolean, default: false },
    snapshot: { type: Object, default: null },
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

<style scoped>
.online-dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: currentColor;
    box-shadow: 0 0 0 0 currentColor;
    animation: online-pulse 1.6s ease-in-out infinite;
}

@keyframes online-pulse {
    0%, 100% {
        box-shadow: 0 0 0 0 rgba(132, 204, 22, 0.6);
    }
    50% {
        box-shadow: 0 0 0 4px rgba(132, 204, 22, 0);
    }
}

@media (prefers-reduced-motion: reduce) {
    .online-dot { animation: none; }
}
</style>
