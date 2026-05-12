<script setup>
import { computed, h, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { GDataTable, GEmpty, GIcon, Loading } from '@gameap/ui'
import GButton from '@/components/GButton.vue'
import { useServerStore } from '@/store/server'
import { useServerTasksStore } from '@/store/serverTasks'
import { confirm, errorNotification } from '@/parts/dialogs'
import { trans } from '@/i18n/i18n'

import ServerTaskForm from './ServerTaskForm.vue'
import ServerTaskExecutionsModal from './ServerTaskExecutionsModal.vue'

const props = defineProps({
    serverId: { type: Number, required: true },
    privileges: {
        type: Object,
        default: () => ({ start: true, stop: true, restart: true, update: true }),
    },
})

const serverStore = useServerStore()
const tasksStore = useServerTasksStore()
const { tasks, loading } = storeToRefs(tasksStore)

const formVisible = ref(false)
const executionsVisible = ref(false)
const selectedTaskIndex = ref(null)
const executionsTask = ref(null)

const COMMAND_BADGE_CLASS = {
    start: 'badge-green',
    stop: 'badge-red',
    restart: 'badge-orange',
    update: 'badge-blue',
    reinstall: 'badge-stone',
}

const COMMAND_ICON = {
    start: 'play',
    stop: 'stop',
    restart: 'restart',
    update: 'refresh',
    reinstall: 'sync',
}

const REPEAT_ENDLESSLY = 0
const REPEAT_ONCE = 1

const selectedTask = computed(() => {
    if (selectedTaskIndex.value === null) return null

    return tasks.value[selectedTaskIndex.value] ?? null
})

function commandLabel(command) {
    if (!command) return '—'
    const key = `servers.${command}`
    const value = trans(key)

    return value === key ? command : value
}

function repeatText(row) {
    const value = parseInt(row.repeat, 10)
    if (value === REPEAT_ENDLESSLY) return trans('servers_tasks.endlessly')
    if (value === REPEAT_ONCE) return trans('servers_tasks.once')

    return String(value)
}

function renderTaskCell(row) {
    const name = row.name && row.name.length > 0 ? row.name : null
    const command = row.command
    const iconName = COMMAND_ICON[command] || 'calendar'
    const badgeClass = COMMAND_BADGE_CLASS[command] || 'badge-stone'

    const left = h(
        'span',
        {
            class:
                `${badgeClass} inline-flex items-center justify-center w-7 h-7 rounded-full flex-shrink-0`,
        },
        h(GIcon, { name: iconName }),
    )

    const right = name
        ? h('div', { class: 'flex flex-col leading-tight' }, [
            h(
                'span',
                { class: 'font-medium text-stone-800 dark:text-stone-100' },
                name,
            ),
            h(
                'span',
                { class: 'text-xs text-stone-500 dark:text-stone-400 font-mono' },
                commandLabel(command),
            ),
        ])
        : h(
            'span',
            { class: 'font-medium text-stone-800 dark:text-stone-100' },
            commandLabel(command),
        )

    return h('div', { class: 'flex items-center gap-3' }, [left, right])
}

function renderScheduleCell(row) {
    const children = [
        h(
            'span',
            { class: 'font-mono text-sm text-stone-800 dark:text-stone-100' },
            row.execute_date || '—',
        ),
    ]
    if (row.timezone) {
        children.push(
            h(
                'span',
                { class: 'text-xs text-stone-500 dark:text-stone-400' },
                row.timezone,
            ),
        )
    }

    return h('div', { class: 'flex flex-col leading-tight' }, children)
}

function renderRepeatCell(row) {
    const children = [
        h(
            'span',
            { class: 'text-sm text-stone-800 dark:text-stone-100' },
            repeatText(row),
        ),
    ]
    if (row.repeat_period && parseInt(row.repeat, 10) !== REPEAT_ONCE) {
        children.push(
            h(
                'span',
                { class: 'text-xs text-stone-500 dark:text-stone-400' },
                row.repeat_period,
            ),
        )
    }

    return h('div', { class: 'flex flex-col leading-tight' }, children)
}

function renderStatusCell(row) {
    const enabled = row.enabled !== false
    const badgeClass = enabled ? 'badge-green' : 'badge-stone'
    const label = enabled
        ? trans('servers_tasks.active')
        : trans('servers_tasks.paused')

    return h('span', { class: badgeClass }, label)
}

function renderActionButton(color, iconName, label, onClick, extraClass = '') {
    return h(
        GButton,
        {
            color,
            size: 'small',
            class: extraClass,
            onClick,
        },
        () => [
            h(GIcon, { name: iconName }),
            h('span', { class: 'hidden lg:inline ml-1' }, label),
        ],
    )
}

function renderActionsCell(row, rowIndex) {
    return h('div', { class: 'flex justify-end' }, [
        renderActionButton('blue', 'edit', trans('main.edit'), () => openEdit(rowIndex), 'mr-1'),
        renderActionButton(
            'black',
            'eye',
            trans('servers_tasks.executions'),
            () => openExecutions(row),
            'mr-1',
        ),
        renderActionButton('red', 'delete', trans('main.delete'), () => deleteTask(rowIndex)),
    ])
}

const columns = computed(() => [
    {
        title: trans('servers_tasks.task'),
        key: 'task',
        render: renderTaskCell,
    },
    {
        title: trans('servers_tasks.schedule'),
        key: 'schedule',
        render: renderScheduleCell,
    },
    {
        title: trans('servers_tasks.repeat'),
        key: 'repeat',
        render: renderRepeatCell,
    },
    {
        title: trans('servers_tasks.status'),
        key: 'status',
        width: 120,
        render: renderStatusCell,
    },
    {
        title: trans('main.actions'),
        key: 'actions',
        align: 'right',
        render: renderActionsCell,
    },
])

function openCreate() {
    selectedTaskIndex.value = null
    formVisible.value = true
}

function openEdit(index) {
    selectedTaskIndex.value = index
    formVisible.value = true
}

function openExecutions(row) {
    executionsTask.value = row
    executionsVisible.value = true
}

function deleteTask(index) {
    confirm(trans('servers_tasks.confirm_remove'), async () => {
        try {
            await tasksStore.destroyTask(index)
        } catch (e) {
            errorNotification(e)
        }
    })
}

function refresh() {
    tasksStore.fetchTasks().catch((e) => errorNotification(e))
}

onMounted(() => {
    serverStore.setServerId(props.serverId)
    refresh()
})
</script>

<template>
    <div id="server-task-component">
        <div class="flex items-center gap-2 mb-3">
            <GButton color="green" size="small" @click="openCreate">
                <GIcon name="add-square" />
                <span class="ml-1">{{ trans('main.add') }}</span>
            </GButton>
            <GButton color="white" size="small" @click="refresh">
                <GIcon name="refresh" />
                <span class="ml-1">{{ trans('servers_tasks.refresh') }}</span>
            </GButton>
        </div>

        <GDataTable
            :columns="columns"
            :data="tasks"
            :loading="loading"
            :row-key="(row) => row.id"
        >
            <template #loading>
                <Loading />
            </template>
            <template #empty>
                <GEmpty :description="trans('servers_tasks.empty_list')" />
            </template>
        </GDataTable>

        <ServerTaskForm
            v-model:show="formVisible"
            :task="selectedTask"
            :task-index="selectedTaskIndex"
            :privileges="privileges"
            @saved="formVisible = false"
        />

        <ServerTaskExecutionsModal
            v-model:show="executionsVisible"
            :task="executionsTask"
        />
    </div>
</template>
