import { defineStore } from 'pinia'
import axios from '../config/axios'

export const useVersionStore = defineStore("version", {
    state: () => ({
        info: {},
        fetched: false,
        apiProcesses: 0,
    }),
    getters: {
        loading: (state) => state.apiProcesses > 0,
        panel: (state) => state.info?.panel || {},
        daemon: (state) => state.info?.daemon || {},
        updateCheckEnabled: (state) => state.info?.update_check_enabled === true,
    },
    actions: {
        async fetchVersion() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/version')
                this.info = response.data
                this.fetched = true
            } finally {
                this.apiProcesses--
            }
        },
        // The dashboard block and the nodes page share one request per session.
        async ensureFetched() {
            if (this.fetched || this.apiProcesses > 0) {
                return
            }

            await this.fetchVersion()
        },
    },
})
