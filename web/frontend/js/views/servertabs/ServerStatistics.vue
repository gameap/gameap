<template>
    <div class="md:flex md:flex-wrap mt-2">
        <div v-if="errorMessage" class="md:w-full">
            <n-alert type="warning" class="mb-3" :title="trans('servers.statistics_error')">
                {{ errorMessage }}
            </n-alert>
        </div>

        <div v-if="!hasAnyData && !errorMessage" class="md:w-full">
            <n-card class="mb-3">
                <Loading />
                <div class="text-center text-stone-500 mt-2">
                    {{ trans('servers.statistics_no_data') }}
                </div>
            </n-card>
        </div>

        <div v-if="hasAnyData" class="md:w-full md:grid md:grid-cols-2 md:gap-4">
            <n-card
                :title="trans('servers.statistics_cpu')"
                class="mb-3"
                header-class="g-card-header"
                :segmented="{ content: true, footer: 'soft' }"
            >
                <v-chart class="h-72 w-full" :option="cpuOption" :update-options="updateOptions" autoresize />
            </n-card>

            <n-card
                :title="trans('servers.statistics_memory')"
                class="mb-3"
                header-class="g-card-header"
                :segmented="{ content: true, footer: 'soft' }"
            >
                <v-chart class="h-72 w-full" :option="memoryOption" :update-options="updateOptions" autoresize />
            </n-card>

            <n-card
                :title="trans('servers.statistics_disk_io')"
                class="mb-3"
                header-class="g-card-header"
                :segmented="{ content: true, footer: 'soft' }"
            >
                <v-chart class="h-72 w-full" :option="diskOption" :update-options="updateOptions" autoresize />
            </n-card>

            <n-card
                :title="trans('servers.statistics_network_io')"
                class="mb-3"
                header-class="g-card-header"
                :segmented="{ content: true, footer: 'soft' }"
            >
                <v-chart class="h-72 w-full" :option="networkOption" :update-options="updateOptions" autoresize />
            </n-card>
        </div>
    </div>
</template>

<script setup>
import { computed } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import {
    GridComponent,
    TooltipComponent,
    LegendComponent,
    DataZoomComponent,
} from 'echarts/components'
import VChart from 'vue-echarts'
import { NCard, NAlert } from 'naive-ui'
import { Loading } from '@gameap/ui'
import { useServerMetricsWebSocket } from '@/composables/useServerMetricsWebSocket'
import { useThemeVars } from '@/utils/theme'
import { trans } from '@/i18n/i18n'

use([
    CanvasRenderer,
    LineChart,
    GridComponent,
    TooltipComponent,
    LegendComponent,
    DataZoomComponent,
])

const props = defineProps({
    serverId: { type: Number, required: true },
})

const { chartPalette, statusColors } = useThemeVars()

const palettePrimary = computed(() => chartPalette.value[6])
const paletteSuccess = computed(() => chartPalette.value[1])
const paletteWarning = computed(() => statusColors.value.warning)
const paletteDanger = computed(() => statusColors.value.danger)

const updateOptions = { replaceMerge: ['series'] }

const {
    errorMessage,
    cpuSeries,
    memoryBytesSeries,
    diskReadSeries,
    diskWriteSeries,
    networkInSeries,
    networkOutSeries,
} = useServerMetricsWebSocket(() => props.serverId)

const hasAnyData = computed(() => {
    return (
        cpuSeries.value.length > 0
        || memoryBytesSeries.value.length > 0
        || diskReadSeries.value.length > 0
        || diskWriteSeries.value.length > 0
        || networkInSeries.value.length > 0
        || networkOutSeries.value.length > 0
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

function pointsToData(points) {
    return points.map(p => [p.ts, p.v])
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
        palette: [palettePrimary.value, paletteSuccess.value, paletteWarning.value],
    })
    opt.series = cpuSeries.value.map(s =>
        makeLineSeries(trans('servers.statistics_cpu'), s.points, { areaStyle: { opacity: 0.1 } }),
    )
    return opt
})

const memoryOption = computed(() => {
    const opt = baseOption({
        yMax: null,
        yFormatter: formatBytes,
        palette: [palettePrimary.value],
    })
    opt.series = memoryBytesSeries.value.map(s =>
        makeLineSeries(trans('servers.statistics_memory'), s.points, { areaStyle: { opacity: 0.1 } }),
    )
    return opt
})

const diskOption = computed(() => {
    const opt = baseOption({
        yFormatter: formatBitrate,
        palette: [paletteSuccess.value, paletteWarning.value],
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
    return opt
})

const networkOption = computed(() => {
    const opt = baseOption({
        yFormatter: formatBitrate,
        palette: [palettePrimary.value, paletteDanger.value],
        showLegend: true,
    })
    const seriesList = []
    for (const s of networkInSeries.value) {
        seriesList.push(makeLineSeries(trans('servers.statistics_in'), s.points))
    }
    for (const s of networkOutSeries.value) {
        seriesList.push(makeLineSeries(trans('servers.statistics_out'), s.points))
    }
    opt.series = seriesList
    return opt
})
</script>
