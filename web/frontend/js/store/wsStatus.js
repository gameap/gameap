import { defineStore } from 'pinia'

export const useWsStatusStore = defineStore('wsStatus', {
    state: () => ({
        connections: {},
    }),
    getters: {
        aggregateStatus: (state) => {
            const list = Object.values(state.connections)
            if (list.length === 0) return 'idle'
            if (list.some(c => c.status === 'connected')) return 'connected'
            if (list.every(c => c.status === 'failed')) return 'failed'

            return 'connecting'
        },
        anyEverConnected: (state) =>
            Object.values(state.connections).some(c => c.everConnected),
        failedFirstConnect: (state) => {
            const list = Object.values(state.connections)

            return list.length > 0
                && list.some(c => c.status === 'failed')
                && !list.some(c => c.everConnected)
        },
        hasDisconnected: (state) =>
            Object.values(state.connections).some(c => c.status === 'disconnected' || c.status === 'failed'),
        hasFailureSignal: (state) =>
            Object.values(state.connections).some(c =>
                c.status === 'disconnected' || c.status === 'failed' || c.attempts > 0,
            ),
    },
    actions: {
        register(id) {
            this.connections[id] = {
                status: 'connecting',
                everConnected: false,
                attempts: 0,
            }
        },
        setStatus(id, status, attempts = 0) {
            const conn = this.connections[id]
            if (!conn) return

            conn.status = status
            conn.attempts = attempts
        },
        markEverConnected(id) {
            const conn = this.connections[id]
            if (!conn) return

            conn.everConnected = true
        },
        unregister(id) {
            delete this.connections[id]
        },
    },
})
