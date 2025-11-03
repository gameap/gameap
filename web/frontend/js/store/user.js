import { defineStore } from 'pinia'
import axios from '../config/axios'

export const useUserStore = defineStore('user', {
    state: () => ({
        userId: 0,
        user: {},
        servers: [],
        permissionsForServer: new Map(),

        // This is a counter to keep track of how many API processes are running
        apiProcesses: 0,
    }),
    getters: {
        loading: (state) => state.apiProcesses > 0,
        getServerPermissions: (state) => (serverId) => state.permissionsForServer.get(serverId),
    },
    actions: {
        setUserId(userId) {
            this.userId = userId;
        },
        async fetchUser() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/users/'+this.userId)
                this.user = response.data;
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async saveUser(user) {
            this.apiProcesses++
            try {
                await axios.put('/api/users/'+this.userId, user)
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async fetchServers() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/users/'+this.userId+'/servers')
                this.servers = response.data;
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async fetchPermissionsForServer(serverId) {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/users/'+this.userId+'/servers/'+serverId+'/permissions')
                this.setServerPermissions(serverId, response.data);
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        setServerPermissions(serverId, permissions) {
            this.permissionsForServer.set(serverId, permissions);
        },
        async savePermissionsForServer(serverId) {
            this.apiProcesses++
            try {
                await axios.put(
                    '/api/users/'+this.userId+'/servers/'+serverId+'/permissions',
                    this.permissionsForServer.get(serverId),
                )
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        }
    },
})