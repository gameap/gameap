<template>
  <div class="space-y-2">
    <div
        v-for="m in mounts"
        :key="m.mount"
        class="flex items-center gap-3"
    >
      <span
          class="text-xs font-mono w-32 truncate text-stone-500 dark:text-stone-400"
          :title="m.mount"
      >
        {{ m.mount }}
      </span>
      <n-progress
          type="line"
          :percentage="clampPct(m.percent)"
          :color="paletteFor(m.percent, chartPalette[0])"
          :height="10"
          :border-radius="2"
          :show-indicator="false"
          class="flex-1"
      />
      <span class="text-xs font-mono tabular-nums w-48 text-right">
        <span>{{ formatBytes(m.used) }} / {{ formatBytes(m.total) }}</span>
        <span v-if="m.percent != null" class="ml-1 text-stone-500">
          ({{ formatPercent(m.percent) }})
        </span>
      </span>
    </div>
  </div>
</template>

<script setup>
import { NProgress } from 'naive-ui'
import { useThemeVars } from '@/utils/theme'

defineProps({
    mounts: { type: Array, required: true },
})

const { chartPalette, statusColors } = useThemeVars()

function clampPct(v) {
    if (v === null || v === undefined || Number.isNaN(v)) return 0
    return Math.max(0, Math.min(100, Number(v)))
}

function paletteFor(v, base) {
    if (v === null || v === undefined) return base
    if (v > 90) return statusColors.value.danger
    if (v > 75) return statusColors.value.warning
    return base
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

function formatPercent(v) {
    if (v === null || v === undefined || Number.isNaN(v)) return '—'
    return `${Number(v).toFixed(1)}%`
}
</script>
