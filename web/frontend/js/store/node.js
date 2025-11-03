import { defineStore } from 'pinia'
import axios from '../config/axios'

export const useNodeStore = defineStore('node', {
    state: () => ({
        nodeId: 0,
        node: {},
        daemonInfo: {},

        // This is a counter to keep track of how many API processes are running
        apiProcesses: 0,
    }),
    getters: {
        loading: (state) => state.apiProcesses > 0,
    },
    actions: {
        setNodeId(nodeId) {
            this.nodeId = nodeId;
        },
        async fetchNode() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/dedicated_servers/'+this.nodeId)
                this.node = response.data;
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async fetchDaemonInfo() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/dedicated_servers/'+this.nodeId+'/daemon')
                this.daemonInfo = response.data;
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async saveNode(node) {
            this.apiProcesses++
            try {
                await axios.put('/api/dedicated_servers/'+this.nodeId, node)
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
    },
})