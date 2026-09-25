import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import axios from '../config/axios'

export const useServerListStore = defineStore('serverList', () => {
    const servers = ref([])
    const currentPage = ref(1)
    const perPage = ref(30)
    const total = ref(0)
    const lastPage = ref(1)
    const summary = ref({
        total: 0,
        online: 0,
        offline: 0,
    })
    const apiProcesses = ref(0)

    const loading = computed(() => apiProcesses.value > 0)

    function buildParams(filter = {}) {
        const params = {
            'page[number]': filter.page ?? currentPage.value ?? 1,
            'page[size]': filter.perPage ?? perPage.value ?? 30,
        }

        if (filter.nodeId) {
            params['filter[ds_id]'] = filter.nodeId
        }

        if (filter.gameIds && filter.gameIds.length > 0) {
            params['filter[game_id]'] = filter.gameIds.join(',')
        }

        if (filter.enabled !== undefined && filter.enabled !== null) {
            params['filter[enabled]'] = filter.enabled ? 'true' : 'false'
        }

        if (filter.sort) {
            params.sort = filter.sort
        }

        return params
    }

    function applyResponse(data) {
        servers.value = data.data
        currentPage.value = data.current_page
        perPage.value = data.per_page
        total.value = data.total
        lastPage.value = data.last_page
    }

    async function fetchServersByFilter(filter = {}) {
        apiProcesses.value++
        try {
            const response = await axios.get('/api/servers', { params: buildParams(filter) })
            applyResponse(response.data)
        } catch (error) {
            if (error.__CANCEL__) {
                return
            }
            throw error
        } finally {
            apiProcesses.value--
        }
    }

    async function fetchServersByNode(nodeId = null, filter = {}) {
        return fetchServersByFilter({ ...filter, nodeId })
    }

    async function fetchServersSummary() {
        apiProcesses.value++
        try {
            const response = await axios.get('/api/servers/summary')
            summary.value = response.data
        } catch (error) {
            if (error.__CANCEL__) {
                return
            }
            throw error
        } finally {
            apiProcesses.value--
        }
    }

    async function create(server, config = {}) {
        apiProcesses.value++
        try {
            await axios.post('/api/servers', server, config)
        } finally {
            apiProcesses.value--
        }
    }

    async function deleteById(id, deleteFiles) {
        apiProcesses.value++
        try {
            await axios.post(
                '/api/servers/' + id,
                {delete_files: deleteFiles},
                {headers: {'X-Http-Method-Override': 'DELETE'}},
            )
        } catch (error) {
            if (error.__CANCEL__) {
                return
            }
            throw error
        } finally {
            apiProcesses.value--
        }
    }

    return {
        servers,
        currentPage,
        perPage,
        total,
        lastPage,
        summary,
        apiProcesses,

        loading,

        fetchServersByFilter,
        fetchServersByNode,
        fetchServersSummary,
        create,
        deleteById,
    }
})
