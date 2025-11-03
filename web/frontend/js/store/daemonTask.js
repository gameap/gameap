import { defineStore } from 'pinia'
import axios from '../config/axios'

export const useDaemonTaskStore = defineStore('daemonTask', {
    state: () => ({
        taskId: 0,
        task: {},

        // This is a counter to keep track of how many API processes are running
        apiProcesses: 0,
    }),
    getters: {
        loading: (state) => state.apiProcesses > 0,
    },
    actions: {
        setTaskId(taskId) {
            this.taskId = taskId;
        },
        async fetchTaskOutput() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/gdaemon_tasks/' + this.taskId + '/output')
                this.task = response.data
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        }
    },
})