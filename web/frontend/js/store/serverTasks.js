import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import axios from '../config/axios'
import { useServerStore } from './server'

dayjs.extend(utc)

const normalizeTask = (task) => ({
    ...task,
    execute_date: dayjs.utc(task.execute_date).local().format('YYYY-MM-DD HH:mm:ss'),
})

const normalizeExecution = (execution) => ({
    ...execution,
    started_at: execution.started_at
        ? dayjs.utc(execution.started_at).local().format('YYYY-MM-DD HH:mm:ss')
        : null,
    finished_at: execution.finished_at
        ? dayjs.utc(execution.finished_at).local().format('YYYY-MM-DD HH:mm:ss')
        : null,
    created_at: execution.created_at
        ? dayjs.utc(execution.created_at).local().format('YYYY-MM-DD HH:mm:ss')
        : null,
    updated_at: execution.updated_at
        ? dayjs.utc(execution.updated_at).local().format('YYYY-MM-DD HH:mm:ss')
        : null,
})

const serializeTaskForSave = (task) => {
    const payload = { ...task }
    if (payload.execute_date) {
        payload.execute_date = dayjs(payload.execute_date).utc().format('YYYY-MM-DD HH:mm:ss')
    }
    return payload
}

export const useServerTasksStore = defineStore('serverTasks', () => {
    const tasks = ref([])
    const executions = ref([])
    const apiProcesses = ref(0)
    const executionsLoading = ref(false)

    const loading = computed(() => apiProcesses.value > 0)

    async function fetchTasks() {
        const serverStore = useServerStore()
        if (serverStore.serverId <= 0) {
            return
        }

        apiProcesses.value++
        try {
            const response = await axios.get('/api/servers/' + serverStore.serverId + '/tasks')
            tasks.value = response.data.map(normalizeTask)
        } finally {
            apiProcesses.value--
        }
    }

    async function storeTask(task) {
        const serverStore = useServerStore()
        const payload = serializeTaskForSave(task)

        apiProcesses.value++
        try {
            const response = await axios.post('/api/servers/' + serverStore.serverId + '/tasks', payload)
            tasks.value.push(normalizeTask(response.data))
        } finally {
            apiProcesses.value--
        }
    }

    async function updateTask(taskIndex, task) {
        const serverStore = useServerStore()
        const taskId = tasks.value[taskIndex].id
        const payload = serializeTaskForSave(task)

        apiProcesses.value++
        try {
            const response = await axios.put(
                '/api/servers/' + serverStore.serverId + '/tasks/' + taskId,
                payload,
            )
            tasks.value[taskIndex] = normalizeTask(response.data)
        } finally {
            apiProcesses.value--
        }
    }

    async function destroyTask(taskIndex) {
        const serverStore = useServerStore()
        if (serverStore.serverId <= 0) {
            return
        }

        const taskId = tasks.value[taskIndex].id

        apiProcesses.value++
        try {
            await axios.delete('/api/servers/' + serverStore.serverId + '/tasks/' + taskId)
            tasks.value.splice(taskIndex, 1)
        } finally {
            apiProcesses.value--
        }
    }

    async function fetchTaskExecutions(taskId, status) {
        const serverStore = useServerStore()
        if (serverStore.serverId <= 0 || !taskId) {
            return
        }

        const params = {}
        if (status) {
            params.status = status
        }

        executionsLoading.value = true
        try {
            const response = await axios.get(
                '/api/servers/' + serverStore.serverId + '/tasks/' + taskId + '/executions',
                { params },
            )
            const data = Array.isArray(response.data?.data) ? response.data.data : []
            executions.value = data.map(normalizeExecution)
        } finally {
            executionsLoading.value = false
        }
    }

    function clearExecutions() {
        executions.value = []
    }

    return {
        tasks,
        executions,
        apiProcesses,
        executionsLoading,
        loading,
        fetchTasks,
        storeTask,
        updateTask,
        destroyTask,
        fetchTaskExecutions,
        clearExecutions,
    }
})
