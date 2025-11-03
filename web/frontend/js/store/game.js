import { defineStore } from 'pinia'
import axios from '../config/axios'

export const useGameStore = defineStore('game', {
    state: () => ({
        loading: false,
        gameCode: '',
        game: {},
        mods: [],
    }),
    actions: {
        setGameCode(gameCode) {
            this.gameCode = gameCode;
        },
        async fetchGame() {
            this.loading = true
            try {
                const response = await axios.get('/api/games/' + this.gameCode)
                this.game = response.data;
            } catch (error) {
                throw error
            } finally {
                this.loading = false
            }
        },
        async fetchMods() {
            this.loading = true
            try {
                const response = await axios.get('/api/games/' + this.gameCode + '/mods')
                this.mods = response.data;
            } catch (error) {
                throw error
            } finally {
                this.loading = false
            }
        },
        async saveGame(game) {
            this.loading = true
            try {
                await axios.put('/api/games/' + this.gameCode, game)
            } catch (error) {
                throw error
            } finally {
                this.loading = false
            }
        }
    },
})