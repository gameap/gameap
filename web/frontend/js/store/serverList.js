import { defineStore } from 'pinia'
import axios from '../config/axios'

export const useServerListStore = defineStore('serverList', {
    state: () => ({
        servers: [],
        summary: {
            total: 0,
            online: 0,
            offline: 0,
        },

        apiProcesses: 0,
    }),
    getters: {
        loading: (state) => state.apiProcesses > 0,
    },
    actions: {
        async fetchServersByFilter(filter) {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/servers')
                this.servers = response.data;
            } catch (error) {
                if (error.__CANCEL__) {
                    return
                }
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async fetchServersSummary() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/servers/summary')
                this.summary = response.data;
            } catch (error) {
                if (error.__CANCEL__) {
                    return
                }
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async create(server) {
            this.apiProcesses++
            try {
                await axios.post('/api/servers', server)
            } catch (error) {
                if (error.__CANCEL__) {
                    return
                }
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async deleteById(id, deleteFiles) {
            this.apiProcesses++
            try {
                await axios.post(
                    '/api/servers/'+id,
                    {delete_files: deleteFiles},
                    {headers: {'X-Http-Method-Override': 'DELETE'}},
                )
            } catch (error) {
                if (error.__CANCEL__) {
                    return
                }
                throw error
            } finally {
                this.apiProcesses--
            }
        }
    },
})