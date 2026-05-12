<script setup>
import { computed, h, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { NSelect, NTooltip } from 'naive-ui'
import { GDataTable, GEmpty, GIcon, GModal, Loading } from '@gameap/ui'
import GButton from '@/components/GButton.vue'
import { useServerTasksStore } from '@/store/serverTasks'
import { useAuthStore } from '@/store/auth'
import { errorNotification } from '@/parts/dialogs'
import { trans } from '@/i18n/i18n'

const props = defineProps({
    show: { type: Boolean, default: false },
    task: { type: Object, default: null },
})

const emit = defineEmits(['update:show'])

const tasksStore = useServerTasksStore()
const authStore = useAuthStore()
const { executions, executionsLoading } = storeToRefs(tasksStore)

const statusFilter = ref(null)

const STATUS_BADGE = {
    running: 'badge-blue',
    success: 'badge-green',
    failed: 'badge-red',
    canceled: 'badge-stone',
    skipped: 'badge-light',
    timed_out: 'badge-orange',
}

const statusOptions = computed(() => [
    { label: trans('servers_tasks.statuses.running'), value: 'running' },
    { label: trans('servers_tasks.statuses.success'), value: 'success' },
    { label: trans('servers_tasks.statuses.failed'), value: 'failed' },
    { label: trans('servers_tasks.statuses.canceled'), value: 'canceled' },
    { label: trans('servers_tasks.statuses.skipped'), value: 'skipped' },
    { label: trans('servers_tasks.statuses.timed_out'), value: 'timed_out' },
])

const isAdmin = computed(() => authStore.isAdmin)

const title = computed(() => {
    if (!props.task) return trans('servers_tasks.execution_history')
    const label = props.task.name || props.task.command || ''

    return `${trans('servers_tasks.execution_history')} — ${label}`
})

function statusLabel(status) {
    const key = `servers_tasks.statuses.${status}`
    const value = trans(key)

    return value === key ? status : value
}

function formatDuration(ms) {
    if (ms == null) return null
    const totalSeconds = Math.max(0, Math.floor(ms / 1000))
    if (totalSeconds < 60) return `${totalSeconds}s`
    const minutes = Math.floor(totalSeconds / 60)
    const seconds = totalSeconds % 60
    if (minutes < 60) return seconds === 0 ? `${minutes}m` : `${minutes}m ${seconds}s`
    const hours = Math.floor(minutes / 60)
    const restMin = minutes % 60

    return restMin === 0 ? `${hours}h` : `${hours}h ${restMin}m`
}

function renderStatus(row) {
    const badgeClass = STATUS_BADGE[row.status] || 'badge-light'

    return h('span', { class: badgeClass }, statusLabel(row.status))
}

function renderDuration(row) {
    if (row.status === 'running') {
        return h(
            'span',
            { class: 'text-sky-600 dark:text-sky-300 italic text-sm' },
            trans('servers_tasks.still_running'),
        )
    }
    const formatted = formatDuration(row.duration_ms)

    return formatted ?? h('span', { class: 'text-stone-400' }, '—')
}

function renderExitCode(row) {
    if (row.exit_code == null) {
        return h('span', { class: 'text-stone-400' }, '—')
    }
    const value = parseInt(row.exit_code, 10)
    const cls = value === 0
        ? 'text-lime-700 dark:text-lime-300 font-mono text-sm'
        : 'text-red-600 dark:text-red-400 font-mono text-sm font-semibold'

    return h('span', { class: cls }, String(value))
}

function renderExecutionId(row) {
    return h(NTooltip, {}, {
        trigger: () => h(
            'span',
            { class: 'font-mono text-xs text-stone-500 dark:text-stone-400 cursor-help' },
            String(row.execution_id || '').slice(0, 8),
        ),
        default: () => row.execution_id,
    })
}

function renderDetailRow(label, value) {
    return h('div', { class: 'flex flex-col gap-1' }, [
        h(
            'span',
            { class: 'text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400' },
            label,
        ),
        h(
            'span',
            { class: 'text-sm text-stone-800 dark:text-stone-100 break-all' },
            value ?? '—',
        ),
    ])
}

function renderExpand(row) {
    const details = [
        renderDetailRow(trans('servers_tasks.execution_id'), row.execution_id),
        renderDetailRow(trans('servers_tasks.task_version'), row.task_version ? `v${row.task_version}` : '—'),
        renderDetailRow(trans('servers_tasks.started_at'), row.started_at),
        renderDetailRow(trans('servers_tasks.finished_at'), row.finished_at),
        renderDetailRow(trans('servers_tasks.duration'), formatDuration(row.duration_ms) || '—'),
        renderDetailRow(
            trans('servers_tasks.exit_code'),
            row.exit_code == null ? '—' : String(row.exit_code),
        ),
    ]

    const children = [
        h(
            'div',
            { class: 'grid grid-cols-2 md:grid-cols-3 gap-x-6 gap-y-3 mb-3' },
            details,
        ),
    ]

    if (row.error_message) {
        children.push(
            h(
                'div',
                {
                    class:
                        'mb-3 p-3 rounded border border-red-300 dark:border-red-900 bg-red-50 dark:bg-red-950/40 text-red-700 dark:text-red-300 text-sm whitespace-pre-wrap font-mono',
                },
                row.error_message,
            ),
        )
    }

    if (isAdmin.value && row.output_inline) {
        children.push(
            h(
                'div',
                { class: 'mb-3' },
                [
                    h(
                        'div',
                        {
                            class:
                                'text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400 mb-1',
                        },
                        trans('servers_tasks.output'),
                    ),
                    h(
                        'pre',
                        {
                            class:
                                'bg-stone-900 text-stone-100 text-xs p-3 rounded max-h-80 overflow-auto whitespace-pre-wrap',
                        },
                        row.output_inline,
                    ),
                ],
            ),
        )
    }

    if (isAdmin.value && row.output_storage_path) {
        children.push(
            h(
                'a',
                {
                    href: row.output_storage_path,
                    target: '_blank',
                    rel: 'noopener',
                    class:
                        'inline-flex items-center gap-2 text-sm text-sky-600 dark:text-sky-300 hover:underline',
                },
                [
                    h(GIcon, { name: 'download' }),
                    trans('servers_tasks.download_full_output'),
                ],
            ),
        )
    }

    return h(
        'div',
        {
            class:
                'p-4 bg-stone-50 dark:bg-stone-900/40 border-t border-stone-200 dark:border-stone-800',
        },
        children,
    )
}

const columns = computed(() => [
    {
        type: 'expand',
        renderExpand,
    },
    {
        title: trans('servers_tasks.status'),
        key: 'status',
        width: 140,
        render: renderStatus,
    },
    {
        title: trans('servers_tasks.command'),
        key: 'command',
        width: 110,
        render: (row) => h('span', { class: 'font-mono text-xs' }, row.command),
    },
    {
        title: trans('servers_tasks.started_at'),
        key: 'started_at',
        render: (row) => row.started_at || '—',
    },
    {
        title: trans('servers_tasks.duration'),
        key: 'duration',
        width: 130,
        render: renderDuration,
    },
    {
        title: trans('servers_tasks.exit_code'),
        key: 'exit_code',
        width: 110,
        render: renderExitCode,
    },
    {
        title: trans('servers_tasks.execution_id'),
        key: 'execution_id',
        width: 130,
        render: renderExecutionId,
    },
])

async function reload() {
    if (!props.task) return
    try {
        await tasksStore.fetchTaskExecutions(props.task.id, statusFilter.value || undefined)
    } catch (e) {
        errorNotification(e)
    }
}

watch(
    () => [props.show, props.task?.id],
    ([open]) => {
        if (open) {
            statusFilter.value = null
            tasksStore.clearExecutions()
            reload()
        } else {
            tasksStore.clearExecutions()
        }
    },
    { immediate: true },
)

watch(statusFilter, () => {
    if (props.show) {
        reload()
    }
})

function close() {
    emit('update:show', false)
}
</script>

<template>
    <GModal
        :show="show"
        :title="title"
        preset="card"
        :bordered="false"
        :style="{ width: '1200px', maxWidth: '95vw' }"
        @update:show="(value) => emit('update:show', value)"
    >
        <div class="flex flex-wrap items-end gap-3 mb-4">
            <div class="flex-1 min-w-[200px] max-w-xs">
                <label class="block text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400 mb-1">
                    {{ trans('servers_tasks.filter_status') }}
                </label>
                <NSelect
                    v-model:value="statusFilter"
                    :options="statusOptions"
                    :placeholder="trans('servers_tasks.all_statuses')"
                    clearable
                />
            </div>
            <GButton color="white" size="small" @click="reload">
                <GIcon name="refresh" />
                <span class="ml-1">{{ trans('servers_tasks.refresh') }}</span>
            </GButton>
        </div>

        <div class="min-h-[480px]">
            <GDataTable
                :columns="columns"
                :data="executions"
                :loading="executionsLoading"
                :row-key="(row) => row.id"
                :single-line="false"
            >
                <template #loading>
                    <Loading />
                </template>
                <template #empty>
                    <GEmpty :description="trans('servers_tasks.no_executions')" />
                </template>
            </GDataTable>
        </div>

        <template #footer>
            <div class="flex justify-end">
                <GButton color="white" @click="close">
                    {{ trans('main.close') }}
                </GButton>
            </div>
        </template>
    </GModal>
</template>
