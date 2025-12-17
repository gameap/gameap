import { provide, computed, type InjectionKey, type ComputedRef } from 'vue'
import { useRoute } from 'vue-router'
import { useDebugStore } from '@/stores/debug'
import { usePluginsStore } from '@/stores/plugins'
import type { ServerData, UserData } from '@gameap/plugin-sdk'

// Injection keys (must match SDK expectations)
const PLUGIN_CONTEXT_KEY = 'pluginContext'
const PLUGIN_I18N_KEY = 'pluginI18n'

export interface PluginRouteInfo {
    name: string | null | undefined
    path: string
    params: Record<string, string>
    query: Record<string, string>
    pluginId: string | null
}

export interface PluginServerInfo {
    id: number | null
    data: ServerData | null
    abilities: string[]
}

export interface PluginContext {
    route: ComputedRef<PluginRouteInfo>
    server: ComputedRef<PluginServerInfo>
    user: ComputedRef<UserData>
    stores: {
        auth: Record<string, unknown>
        server: Record<string, unknown>
        plugins: ReturnType<typeof usePluginsStore>
    }
}

export interface PluginI18nContext {
    trans: (key: string, params?: Record<string, string | number>) => string
    locale: string
}

export function providePluginContext(customContext: Partial<PluginContext> = {}): PluginContext {
    const debugStore = useDebugStore()
    const pluginsStore = usePluginsStore()
    const route = useRoute()

    const context: PluginContext = {
        route: computed(() => ({
            name: route.name as string | null | undefined,
            path: route.path,
            params: route.params as Record<string, string>,
            query: route.query as Record<string, string>,
            pluginId: debugStore.currentPluginId,
        })),

        server: computed(() => ({
            id: debugStore.currentServer?.id || null,
            data: debugStore.currentServer,
            abilities: debugStore.currentAbilities,
        })),

        user: computed(() => debugStore.currentUser),

        stores: {
            auth: {}, // Mock auth store
            server: {}, // Mock server store
            plugins: pluginsStore,
        },

        ...customContext,
    }

    provide(PLUGIN_CONTEXT_KEY, context)

    // Provide i18n context
    const i18nContext = createI18nContext(pluginsStore, debugStore.currentPluginId)
    provide(PLUGIN_I18N_KEY, i18nContext)

    return context
}

function createI18nContext(
    pluginsStore: ReturnType<typeof usePluginsStore>,
    pluginId: string | null
): PluginI18nContext {
    const locale = window.gameapLang || 'en'

    const trans = (key: string, params?: Record<string, string | number>): string => {
        const translations = pluginId ? pluginsStore.getPluginTranslations(pluginId) : null
        const langTrans = translations?.[locale] || translations?.['en'] || {}
        let value = langTrans[key] ?? key

        if (params) {
            Object.entries(params).forEach(([k, v]) => {
                value = value.replace(`:${k}`, String(v))
            })
        }

        return value
    }

    return { trans, locale }
}

// Component wrapper that provides context
export { PLUGIN_CONTEXT_KEY, PLUGIN_I18N_KEY }
