import { defineStore } from 'pinia'
import axios from '../config/axios'
import { join } from 'lodash-es'

export const useDaemonTaskListStore = defineStore('daemonTaskList', {
    state: () => ({
        daemonTaskList: [],
        currentPage: 1,
        total: 0,
        lastPage: 1,

        // This is a counter to keep track of how many API processes are running
        apiProcesses: 0,
    }),
    getters: {
        loading: (state) => state.apiProcesses > 0,
    },
    actions: {
        async fetchTasksByFilter(filter) {
            this.apiProcesses++
            try {
                let params = {
                    sort: '-id',
                }

                if (filter.page) {
                    params['page[number]'] = filter.page
                }

                if (filter.tasks) {
                    params['filter[task]'] = join(filter.tasks, ',')
                }

                if (filter.statuses) {
                    params['filter[status]'] = join(filter.statuses, ',')
                }

                if (filter.nodes) {
                    params['filter[dedicated_server_id]'] = join(filter.nodes, ',')
                }

                if (filter.servers) {
                    params['filter[server_id]'] = join(filter.servers, ',')
                }

                const response = await axios.get('/api/gdaemon_tasks', {
                    params: params,
                })
                this.daemonTaskList = response.data.data
                this.currentPage = response.data.current_page
                this.total = response.data.total
                this.lastPage = response.data.last_page
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
    },
})