import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import { serverMocks, serverAbilities } from '@/mocks/servers'
import { userMocks } from '@/mocks/users'
import type { ServerData, UserData } from '@gameap/plugin-sdk'

export type UserType = 'admin' | 'user' | 'guest'
export type ServerType = 'none' | 'minecraft' | 'cs'
export type Locale = 'en' | 'ru'

export interface DebugEvent {
    type: string
    payload: unknown
    timestamp: Date
}

export interface PluginInfo {
    id: string
    name: string
    version: string
}

export interface RouteInfo {
    name: string
    path: string
    params: Record<string, string>
    query: Record<string, string>
}

export const useDebugStore = defineStore('debug', () => {
    // State
    const userType = ref<UserType>('admin')
    const serverType = ref<ServerType>('minecraft')
    const locale = ref<Locale>((window.gameapLang as Locale) || 'en')
    const pluginInfo = ref<PluginInfo | null>(null)
    const currentPluginId = ref<string | null>(null)
    const currentRoute = ref<RouteInfo | null>(null)
    const eventLog = ref<DebugEvent[]>([])
    const debugPanelCollapsed = ref(false)

    // Computed
    const currentUser = computed<UserData>(() => userMocks[userType.value])

    const currentServer = computed<ServerData | null>(() => {
        if (serverType.value === 'none') return null
        return serverMocks[serverType.value]
    })

    const currentAbilities = computed<string[]>(() => {
        if (serverType.value === 'none') return []
        return serverAbilities[serverType.value] || []
    })

    const isDarkMode = computed(() => {
        return document.documentElement.classList.contains('dark')
    })

    // Actions
    function setPluginInfo(info: PluginInfo) {
        pluginInfo.value = info
        currentPluginId.value = info.id
    }

    function setRoute(route: RouteInfo) {
        currentRoute.value = route
    }

    function logEvent(type: string, payload: unknown) {
        eventLog.value.unshift({
            type,
            payload,
            timestamp: new Date(),
        })
        // Keep only last 50 events
        if (eventLog.value.length > 50) {
            eventLog.value.pop()
        }
    }

    function clearEventLog() {
        eventLog.value = []
    }

    function toggleDarkMode() {
        document.documentElement.classList.toggle('dark')
        const isDark = document.documentElement.classList.contains('dark')
        localStorage.setItem('gameap-theme', isDark ? 'dark' : 'light')
    }

    function toggleDebugPanel() {
        debugPanelCollapsed.value = !debugPanelCollapsed.value
    }

    // Watch locale changes
    watch(locale, (newLocale) => {
        window.gameapLang = newLocale
        localStorage.setItem('gameap-locale', newLocale)
    })

    return {
        // State
        userType,
        serverType,
        locale,
        pluginInfo,
        currentPluginId,
        currentRoute,
        eventLog,
        debugPanelCollapsed,

        // Computed
        currentUser,
        currentServer,
        currentAbilities,
        isDarkMode,

        // Actions
        setPluginInfo,
        setRoute,
        logEvent,
        clearEventLog,
        toggleDarkMode,
        toggleDebugPanel,
    }
})
