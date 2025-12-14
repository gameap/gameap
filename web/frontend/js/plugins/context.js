import { inject, provide, computed } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '../store/auth'
import { useServerStore } from '../store/server'
import { usePluginsStore } from '../store/plugins'

const PLUGIN_CONTEXT_KEY = 'pluginContext'
const PLUGIN_I18N_KEY = 'pluginI18n'

export function providePluginContext(customContext = {}) {
    const context = createPluginContext(customContext)
    provide(PLUGIN_CONTEXT_KEY, context)

    const i18nContext = createI18nContext()
    provide(PLUGIN_I18N_KEY, i18nContext)

    return context
}

export function usePluginContext() {
    const injected = inject(PLUGIN_CONTEXT_KEY, null)
    if (injected) return injected

    return createPluginContext()
}

function createPluginContext(customContext = {}) {
    const route = useRoute()
    const authStore = useAuthStore()
    const serverStore = useServerStore()
    const pluginsStore = usePluginsStore()

    return {
        route: computed(() => ({
            name: route.name,
            path: route.path,
            params: route.params,
            query: route.query,
            pluginId: route.meta?.pluginId
        })),

        server: computed(() => ({
            id: serverStore.serverId,
            data: serverStore.server,
            abilities: serverStore.abilities
        })),

        user: computed(() => ({
            id: authStore.profile?.id,
            login: authStore.profile?.login ?? '',
            name: authStore.profile?.name ?? '',
            roles: authStore.profile?.roles ?? [],
            isAdmin: authStore.isAdmin,
            isAuthenticated: authStore.isAuthenticated
        })),

        stores: {
            auth: authStore,
            server: serverStore,
            plugins: pluginsStore
        },

        ...customContext
    }
}

function createI18nContext() {
    const route = useRoute()
    const pluginsStore = usePluginsStore()
    const locale = window.gameapLang || 'en'

    const trans = (key, params) => {
        const pluginId = route.meta?.pluginId
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
