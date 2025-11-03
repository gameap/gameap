import { defineStore } from 'pinia'
import axios from '../config/axios'

export const useGameListStore = defineStore('games', {
    state: () => ({
        games: [],
        gameMods: [],
        allGameMods: [],

        apiProcesses: 0,
    }),
    getters: {
        loading: (state) => state.apiProcesses > 0,
    },
    actions: {
        async fetchGames() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/games')
                this.games = response.data;
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async fetchGameMods(gameId) {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/games/' + gameId + '/mods')
                this.gameMods = response.data;
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async fetchAllGameMods() {
            this.apiProcesses++
            try {
                const response = await axios.get('/api/game_mods')
                this.allGameMods = response.data;
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async createGame(game) {
            this.apiProcesses++
            try {
                const response = await axios.post('/api/games', game)
                return response.data
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async createGameMod(mod) {
            this.apiProcesses++
            try {
                const response = await axios.post('/api/game_mods', mod)
                return response.data
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async upgradeGames() {
            this.apiProcesses++
            try {
                await axios.post(
                    '/api/games/upgrade',
                    {headers: {'X-Http-Method-Override': 'PATCH'}},
                )
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async deleteGameByCode(code) {
            this.apiProcesses++
            try {
                await axios.delete('/api/games/' + code)
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        },
        async deleteModById(id) {
            this.apiProcesses++
            try {
                await axios.delete('/api/game_mods/' + id)
            } catch (error) {
                throw error
            } finally {
                this.apiProcesses--
            }
        }
    },
})