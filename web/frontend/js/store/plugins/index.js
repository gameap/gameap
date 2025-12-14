import { defineStore } from 'pinia'
import { ref, reactive, computed, h } from 'vue'
import PluginRouteWrapper from '../../plugins/components/PluginRouteWrapper.vue'

const SUPPORTED_API_VERSIONS = ['1.0']

export const usePluginsStore = defineStore('plugins', () => {
    // State
    const plugins = ref(new Map())
    const slots = reactive({
        'server-tabs': [],
        'dashboard-widgets': [],
        'sidebar-sections': [],
        'admin-pages': [],
        'home-buttons': []
    })
    const pendingRoutes = ref([])
    const registeredRoutes = ref([])
    const menuItems = reactive({
        servers: [],
        admin: [],
        custom: []
    })
    const loading = ref(false)
    const initialized = ref(false)
    const loadErrors = ref([])
    const pluginTranslations = ref(new Map())

    // Getters
    const isLoading = computed(() => loading.value)
    const isInitialized = computed(() => initialized.value)
    const hasErrors = computed(() => loadErrors.value.length > 0)
    const enabledPlugins = computed(() => {
        return Array.from(plugins.value.values()).filter(p => p.enabled)
    })

    function getSlotComponents(slotName) {
        return slots[slotName] || []
    }

    function getMenuItems(section) {
        return menuItems[section] || []
    }

    function getPlugin(id) {
        return plugins.value.get(id)
    }

    // Actions
    function checkApiVersion(version) {
        return SUPPORTED_API_VERSIONS.includes(version)
    }

    function registerPlugin(pluginDef) {
        if (!checkApiVersion(pluginDef.apiVersion || '1.0')) {
            throw new Error(`Unsupported API version: ${pluginDef.apiVersion}`)
        }

        plugins.value.set(pluginDef.id, {
            id: pluginDef.id,
            name: pluginDef.name,
            version: pluginDef.version,
            apiVersion: pluginDef.apiVersion || '1.0',
            enabled: true,
            loadError: null,
            loadedAt: new Date()
        })

        return pluginDef.id
    }

    function registerSlotComponent(slotName, pluginId, component, options = {}) {
        if (!slots[slotName]) {
            console.warn(`Unknown slot: ${slotName}`)
            return
        }

        slots[slotName].push({
            pluginId,
            component,
            props: options.props || {},
            order: options.order || 0,
            label: options.label || '',
            icon: options.icon || null,
            name: options.name || ''
        })

        slots[slotName].sort((a, b) => a.order - b.order)
    }

    function registerMenuItem(section, pluginId, item) {
        const targetSection = menuItems[section] ? section : 'custom'

        menuItems[targetSection].push({
            pluginId,
            icon: item.icon || 'fas fa-puzzle-piece',
            text: item.text,
            route: item.route,
            order: item.order || 100,
            section: item.section
        })

        menuItems[targetSection].sort((a, b) => a.order - b.order)
    }

    function addPendingRoute(pluginId, route) {
        pendingRoutes.value.push({
            pluginId,
            ...route,
            path: `/plugins/${pluginId}${route.path}`,
            name: `plugin.${pluginId}.${route.name || 'index'}`,
            meta: {
                ...route.meta,
                pluginId,
                requiresAuth: route.meta?.requiresAuth !== false
            }
        })
    }

    function registerRoutes(router) {
        for (const route of pendingRoutes.value) {
            try {
                const originalComponent = route.component
                const wrappedComponent = {
                    name: `PluginRoute_${route.name}`,
                    render() {
                        return h(PluginRouteWrapper, {
                            routeComponent: originalComponent
                        })
                    }
                }

                router.addRoute({
                    path: route.path,
                    name: route.name,
                    component: wrappedComponent,
                    meta: route.meta
                })
                registeredRoutes.value.push(route.name)
            } catch (error) {
                console.error(`Failed to register route ${route.path}:`, error)
            }
        }
        pendingRoutes.value = []
    }

    function setLoading(value) {
        loading.value = value
    }

    function setInitialized(value) {
        initialized.value = value
    }

    function addLoadError(error) {
        loadErrors.value.push(error)
    }

    function setPluginTranslations(pluginId, translations) {
        pluginTranslations.value.set(pluginId, translations)
    }

    function getPluginTranslations(pluginId) {
        return pluginTranslations.value.get(pluginId)
    }

    function resolvePluginText(pluginId, text) {
        if (typeof text !== 'string' || !text.startsWith('@:')) {
            return text
        }
        const key = text.slice(2)
        const translations = pluginTranslations.value.get(pluginId)
        const locale = window.gameapLang || 'en'
        const langTrans = translations?.[locale] || translations?.['en'] || {}
        return langTrans[key] ?? key
    }

    function unregisterPlugin(pluginId) {
        plugins.value.delete(pluginId)

        for (const slotName of Object.keys(slots)) {
            slots[slotName] = slots[slotName].filter(c => c.pluginId !== pluginId)
        }

        for (const section of Object.keys(menuItems)) {
            menuItems[section] = menuItems[section].filter(i => i.pluginId !== pluginId)
        }
    }

    return {
        // State
        plugins,
        slots,
        pendingRoutes,
        registeredRoutes,
        menuItems,
        loading,
        initialized,
        loadErrors,
        pluginTranslations,

        // Getters
        isLoading,
        isInitialized,
        hasErrors,
        enabledPlugins,
        getSlotComponents,
        getMenuItems,
        getPlugin,
        getPluginTranslations,
        resolvePluginText,

        // Actions
        checkApiVersion,
        registerPlugin,
        registerSlotComponent,
        registerMenuItem,
        addPendingRoute,
        registerRoutes,
        setLoading,
        setInitialized,
        addLoadError,
        setPluginTranslations,
        unregisterPlugin
    }
})
