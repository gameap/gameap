<template>
  <div>
    <n-alert v-if="errorMessage" type="warning" class="mb-3" :title="trans('servers.statistics_error')">
      {{ errorMessage }}
    </n-alert>

    <div v-if="!hasAnyData && !errorMessage">
      <GCard class="mb-3">
        <Loading />
        <div class="text-center text-stone-500 mt-2">
          {{ trans('servers.statistics_no_data') }}
        </div>
      </GCard>
    </div>

    <div v-if="hasAnyData" class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <GCard
          v-if="diskMounts.length"
          :title="trans('servers.statistics_disk_usage')"
          class="mb-3 md:col-span-2"
      >
        <DiskUsageList :mounts="diskMounts" />
      </GCard>

      <GCard
          v-if="cpuSeries.length"
          :title="trans('servers.statistics_cpu')"
          class="mb-3"
      >
        <v-chart class="h-56 lg:h-64 w-full" :option="cpuOption" :update-options="updateOptions" autoresize />
      </GCard>

      <GCard
          v-if="memoryBytesSeries.length"
          :title="trans('servers.statistics_memory')"
          class="mb-3"
      >
        <v-chart class="h-56 lg:h-64 w-full" :option="memoryOption" :update-options="updateOptions" autoresize />
      </GCard>

      <GCard
          v-if="hasLoadData"
          :title="trans('servers.statistics_load_average')"
          class="mb-3"
      >
        <v-chart
            class="h-56 lg:h-64 w-full"
            :option="loadOption"
            :update-options="updateOptions"
            autoresize
            @legendselectchanged="(p) => onLegendSelectChanged('load', p)"
        />
      </GCard>

      <GCard
          v-if="hasDiskIOData"
          :title="trans('servers.statistics_disk_io')"
          class="mb-3"
      >
        <v-chart
            class="h-56 lg:h-64 w-full"
            :option="diskOption"
            :update-options="updateOptions"
            autoresize
            @legendselectchanged="(p) => onLegendSelectChanged('disk', p)"
        />
      </GCard>

      <div
          v-if="networkInSeries.length || networkOutSeries.length"
          class="md:col-span-2 grid grid-cols-1 md:grid-cols-2 gap-4"
      >
        <GCard
            v-if="networkInSeries.length"
            :title="trans('servers.statistics_network_receive')"
            class="mb-0"
        >
          <v-chart
              class="h-56 lg:h-64 w-full"
              :option="networkInOption"
              :update-options="updateOptions"
              autoresize
              @legendselectchanged="(p) => onLegendSelectChanged('networkIn', p)"
          />
        </GCard>

        <GCard
            v-if="networkOutSeries.length"
            :title="trans('servers.statistics_network_transmit')"
            class="mb-0"
        >
          <v-chart
              class="h-56 lg:h-64 w-full"
              :option="networkOutOption"
              :update-options="updateOptions"
              autoresize
              @legendselectchanged="(p) => onLegendSelectChanged('networkOut', p)"
          />
        </GCard>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import {
    GridComponent,
    TooltipComponent,
    LegendComponent,
} from 'echarts/components'
import VChart from 'vue-echarts'
import { NAlert } from 'naive-ui'
import { Loading, GCard } from '@gameap/ui'
import { useNodeMetricsWebSocket } from '@/composables/useNodeMetricsWebSocket'
import { useThemeVars } from '@/utils/theme'
import { trans } from '@/i18n/i18n'
import DiskUsageList from '@/components/blocks/DiskUsageList.vue'

use([
    CanvasRenderer,
    LineChart,
    GridComponent,
    TooltipComponent,
    LegendComponent,
])

const props = defineProps({
    nodeId: { type: [Number, String], required: true },
})

const { chartPalette } = useThemeVars()

const paletteCyan = computed(() => chartPalette.value[0])
const paletteLime = computed(() => chartPalette.value[1])
const paletteMagenta = computed(() => chartPalette.value[2])
const paletteDanger = computed(() => chartPalette.value[4])

const updateOptions = { replaceMerge: ['series'] }

const legendSelections = ref({})

function onLegendSelectChanged(chartKey, params) {
    legendSelections.value = {
        ...legendSelections.value,
        [chartKey]: { ...params.selected },
    }
}

function applyLegendSelection(opt, chartKey) {
    const sel = legendSelections.value[chartKey]
    if (sel && opt.legend && opt.legend.show !== false) {
        opt.legend = { ...opt.legend, selected: sel }
    }
}

const {
    errorMessage,
    cpuSeries,
    memoryBytesSeries,
    diskReadSeries,
    diskWriteSeries,
    diskUsageBytesSeries,
    diskUsagePercentSeries,
    diskTotalBytesSeries,
    networkInSeries,
    networkOutSeries,
    load1Series,
    load5Series,
    load15Series,
} = useNodeMetricsWebSocket(() => Number(props.nodeId))

const hasLoadData = computed(() =>
    load1Series.value.length > 0
    || load5Series.value.length > 0
    || load15Series.value.length > 0,
)

const hasDiskIOData = computed(() =>
    diskReadSeries.value.length > 0
    || diskWriteSeries.value.length > 0,
)

const diskMounts = computed(() => {
    const map = new Map()
    const upsert = (mount, patch) => {
        const cur = map.get(mount) || { mount }
        map.set(mount, { ...cur, ...patch })
    }
    const lastVal = (s) => (s.points.length ? s.points[s.points.length - 1].v : null)

    for (const s of diskUsagePercentSeries.value) {
        upsert(s.labels?.mount || '/', { percent: lastVal(s) })
    }
    for (const s of diskUsageBytesSeries.value) {
        upsert(s.labels?.mount || '/', { used: lastVal(s) })
    }
    for (const s of diskTotalBytesSeries.value) {
        upsert(s.labels?.mount || '/', { total: lastVal(s) })
    }
    return Array.from(map.values()).sort((a, b) => a.mount.localeCompare(b.mount))
})

const hasAnyData = computed(() => {
    return (
        cpuSeries.value.length > 0
        || memoryBytesSeries.value.length > 0
        || diskReadSeries.value.length > 0
        || diskWriteSeries.value.length > 0
        || networkInSeries.value.length > 0
        || networkOutSeries.value.length > 0
        || hasLoadData.value
        || diskMounts.value.length > 0
    )
})

function formatBytes(v) {
    if (v == null || Number.isNaN(v)) return ''
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
    if (v == null || Number.isNaN(v)) return ''
    return `${Number(v).toFixed(1)}%`
}

function formatBitrate(v) {
    return `${formatBytes(v)}/s`
}

function formatLoad(v) {
    if (v == null || Number.isNaN(v)) return ''
    return Number(v).toFixed(2)
}

function pointsToData(points) {
    return points.map(p => [p.ts, p.v])
}

function interfaceLabel(s) {
    return s.labels?.interface || trans('servers.statistics_all')
}

function baseOption({ yMax = null, yFormatter, palette, showLegend = false }) {
    return {
        animation: false,
        grid: {
            left: 12,
            right: 16,
            top: showLegend ? 28 : 12,
            bottom: 32,
            containLabel: true,
        },
        tooltip: {
            trigger: 'axis',
            valueFormatter: yFormatter,
        },
        legend: showLegend ? { type: 'scroll', top: 0 } : { show: false },
        xAxis: {
            type: 'time',
        },
        yAxis: {
            type: 'value',
            min: 0,
            max: yMax,
            axisLabel: { formatter: yFormatter },
        },
        color: palette,
    }
}

function makeLineSeries(name, points, extra = {}) {
    return {
        name,
        type: 'line',
        showSymbol: false,
        sampling: 'lttb',
        smooth: false,
        data: pointsToData(points),
        ...extra,
    }
}

const cpuOption = computed(() => {
    const opt = baseOption({
        yMax: 100,
        yFormatter: formatPercent,
        palette: [paletteLime.value],
    })
    opt.series = cpuSeries.value.map(s =>
        makeLineSeries(trans('servers.statistics_cpu'), s.points, { areaStyle: { opacity: 0.12 } }),
    )
    return opt
})

const memoryOption = computed(() => {
    const opt = baseOption({
        yMax: null,
        yFormatter: formatBytes,
        palette: [paletteCyan.value],
    })
    opt.series = memoryBytesSeries.value.map(s =>
        makeLineSeries(trans('servers.statistics_memory'), s.points, { areaStyle: { opacity: 0.12 } }),
    )
    return opt
})

const diskOption = computed(() => {
    const opt = baseOption({
        yFormatter: formatBitrate,
        palette: [paletteLime.value, paletteMagenta.value],
        showLegend: true,
    })
    const seriesList = []
    for (const s of diskReadSeries.value) {
        seriesList.push(makeLineSeries(trans('servers.statistics_read'), s.points))
    }
    for (const s of diskWriteSeries.value) {
        seriesList.push(makeLineSeries(trans('servers.statistics_write'), s.points))
    }
    opt.series = seriesList
    applyLegendSelection(opt, 'disk')
    return opt
})

const networkInOption = computed(() => {
    const opt = baseOption({
        yFormatter: formatBitrate,
        palette: chartPalette.value,
        showLegend: true,
    })
    opt.series = networkInSeries.value
        .slice()
        .sort((a, b) => interfaceLabel(a).localeCompare(interfaceLabel(b)))
        .map(s => makeLineSeries(interfaceLabel(s), s.points))
    applyLegendSelection(opt, 'networkIn')
    return opt
})

const networkOutOption = computed(() => {
    const opt = baseOption({
        yFormatter: formatBitrate,
        palette: chartPalette.value,
        showLegend: true,
    })
    opt.series = networkOutSeries.value
        .slice()
        .sort((a, b) => interfaceLabel(a).localeCompare(interfaceLabel(b)))
        .map(s => makeLineSeries(interfaceLabel(s), s.points))
    applyLegendSelection(opt, 'networkOut')
    return opt
})

const loadOption = computed(() => {
    const opt = baseOption({
        yFormatter: formatLoad,
        palette: [paletteLime.value, paletteCyan.value, paletteMagenta.value, paletteDanger.value],
        showLegend: true,
    })
    const seriesList = []
    for (const s of load1Series.value) {
        seriesList.push(makeLineSeries('1m', s.points))
    }
    for (const s of load5Series.value) {
        seriesList.push(makeLineSeries('5m', s.points))
    }
    for (const s of load15Series.value) {
        seriesList.push(makeLineSeries('15m', s.points))
    }
    opt.series = seriesList
    applyLegendSelection(opt, 'load')
    return opt
})
</script>
