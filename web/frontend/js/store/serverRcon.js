import { defineStore } from 'pinia'
import axios from '../config/axios'

import { useServerStore } from './server.js'

export const useServerRconStore = defineStore('serverRcon', {
    state: () => ({
        fastRcon: [],
        rconSupportedFeatures: {
            rcon: false,
            playersManage: false,
        },
        output: '',

        apiProcesses: 0,
    }),
    getters: {
        loading: (state) => state.apiProcesses > 0,
        serverId: () => {
            const serverStore = useServerStore()

            return serverStore.serverId
        },
        canUseRcon() {
            const serverStore = useServerStore()

            return Boolean(serverStore.abilities['game-server-rcon-console'])
        },
        canManageRconPlayers() {
            const serverStore = useServerStore()

            return Boolean(serverStore.abilities['game-server-rcon-players'])
        },
    },
    actions: {
        setServerId(serverId) {
            const serverStore = useServerStore()

            serverStore.setServerId(serverId)
        },
        async fetchRconSupportedFeatures() {
            const serverStore = useServerStore()
            this.apiProcesses++

            try {
                const response = await axios.get('/api/servers/' + serverStore.serverId + '/rcon/features')
                this.rconSupportedFeatures = response.data;
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async fetchFastRcon() {
            const serverStore = useServerStore()

            this.apiProcesses++

            try {
                const response = await axios.get('/api/servers/' + serverStore.serverId + '/rcon/fast_rcon')
                this.fastRcon = response.data;
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async sendCommand(command) {
            const serverStore = useServerStore()

            this.apiProcesses++

            try {
                const response = await axios.post('/api/servers/' + serverStore.serverId + '/rcon', {
                    command: command
                });
                this.output = response.data.output;
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
    },
})