import { defineStore } from 'pinia'
import axios from '../config/axios'

export const useTokensStore = defineStore("tokens",{
    state: () => ({
        tokens: [],
        abilities: {},

        apiProcesses: 0,
    }),
    getters: {
        loading: (state) => state.apiProcesses > 0,
    },
    actions: {
        async fetchTokens() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/tokens')
                // Ensure we always have an array
                this.tokens = Array.isArray(response.data) ? response.data : []
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async fetchAbilities() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/tokens/abilities')
                this.abilities = response.data;
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async createToken(name, abilities) {
            this.apiProcesses++
            try {
                const response = await axios.post('/api/tokens', {
                    token_name: name,
                    abilities: abilities,
                })
                return response.data
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async deleteToken(id) {
            this.apiProcesses++
            try {
                await axios.delete('/api/tokens/'+id)
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
    },
})