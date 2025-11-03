import { defineStore } from 'pinia'

export const useServerFiltersStore = defineStore('serverFilters', {
    state: () => ({
        selectedGame: null,
        selectedIP: null,
    }),
    getters: {
        hasFilters: (state) => state.selectedGame !== null || state.selectedIP !== null,
    },
    actions: {
        setGameFilter(gameNames) {
            this.selectedGame = gameNames
        },
        setIPFilter(ips) {
            this.selectedIP = ips
        },
        clearFilters() {
            this.selectedGame = null
            this.selectedIP = null
        },
    },
})