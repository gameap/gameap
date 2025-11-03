import { defineStore } from 'pinia'

const STORAGE_KEY = 'gameap_ui_settings'

function loadSettings() {
    try {
        const stored = localStorage.getItem(STORAGE_KEY)
        if (stored) {
            return JSON.parse(stored)
        }
    } catch (e) {
        console.error('Failed to load UI settings from localStorage:', e)
    }
    return {}
}

function saveSettings(settings) {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(settings))
    } catch (e) {
        console.error('Failed to save UI settings to localStorage:', e)
    }
}

const savedSettings = loadSettings()

export const useUISettingsStore = defineStore('uiSettings', {
    state: () => ({
        language: savedSettings.language || null,
        leftMenuMinimized: savedSettings.leftMenuMinimized || false,
        theme: savedSettings.theme || 'light',
    }),
    getters: {
        currentLanguage: (state) => state.language,
        isMenuMinimized: (state) => state.leftMenuMinimized,
        currentTheme: (state) => state.theme,
    },
    actions: {
        setLanguage(lang) {
            this.language = lang
            this._saveToStorage()
        },
        setMenuMinimized(minimized) {
            this.leftMenuMinimized = minimized
            this._saveToStorage()
        },
        toggleMenuMinimized() {
            this.leftMenuMinimized = !this.leftMenuMinimized
            this._saveToStorage()
        },
        setTheme(theme) {
            this.theme = theme
            this._saveToStorage()
        },
        toggleTheme() {
            this.theme = this.theme === 'dark' ? 'light' : 'dark'
            this._saveToStorage()
        },
        _saveToStorage() {
            saveSettings({
                language: this.language,
                leftMenuMinimized: this.leftMenuMinimized,
                theme: this.theme,
            })
        },
    },
})