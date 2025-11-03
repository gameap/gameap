import { defineStore } from 'pinia'
import axios from '../config/axios'

export const useNodeListStore = defineStore("nodeList",{
    state: () => ({
        nodes: [],
        summary: {},

        autoSetupData: {
            link: '',
            token: '',
            host: '',
        },

        apiProcesses: 0,
    }),
    getters: {
        loading: (state) => state.apiProcesses > 0,
    },
    actions: {
        async fetchNodesByFilter(filter) {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/dedicated_servers')
                this.nodes = response.data;
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async fetchNodesSummary() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/dedicated_servers/summary')
                this.summary = response.data;
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async createNode(node) {
            this.apiProcesses++
            try {
                await axios.post('/api/dedicated_servers', node)
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async deleteNode(id) {
            this.apiProcesses++
            try {
                await axios.delete('/api/dedicated_servers/'+id)
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async fetchAutoSetupData() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/dedicated_servers/setup')
                this.autoSetupData = response.data
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
    },
})