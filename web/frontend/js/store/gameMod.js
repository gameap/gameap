import { defineStore } from 'pinia'
import axios from '../config/axios'

export const useGameModStore = defineStore('gameMod', {
    state: () => ({
        loading: false,
        modId: '',
        mod: {},
    }),
    actions: {
        setModId(modId) {
            this.modId = modId;
        },
        async fetchMod() {
            this.loading = true
            try {
                const response = await axios.get('/api/game_mods/' + this.modId)
                this.mod = response.data;
            } catch (error) {
                throw error
            } finally {
                this.loading = false
            }
        },
        async saveMod(mod) {
            this.loading = true
            try {
                await axios.put('/api/game_mods/' + this.modId, mod)
            } catch (error) {
                throw error
            } finally {
                this.loading = false
            }
        },
    },
})