import { defineStore } from 'pinia'

export const useAuthSettingsStore = defineStore('authSettings', {
    state: () => ({
        theme: 'light',
    }),
    getters: {
        currentTheme: (state) => state.theme,
    },
    actions: {
        setTheme(theme) {
            this.theme = theme
        },
    },
})