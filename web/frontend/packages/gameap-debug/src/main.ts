import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHistory } from 'vue-router'
import * as Vue from 'vue'
import * as VueRouter from 'vue-router'
import * as Pinia from 'pinia'

import './styles/main.css'
// Import plugin CSS
// @ts-expect-error - Plugin path is configured via Vite alias
import '@plugin/hex-editor-plugin.css'
import App from './App.vue'
import { useDebugStore } from './stores/debug'
import { usePluginsStore } from './stores/plugins'
import type { PluginDefinition } from '@gameap/plugin-sdk'

// Setup globals for plugin compatibility
function setupGlobals() {
    window.Vue = Vue
    window.VueRouter = VueRouter
    window.Pinia = Pinia

    // Mock axios
    window.axios = {
        get: async (url: string) => {
            console.log('[Mock Axios] GET', url)
            return { data: null }
        },
        post: async (url: string, data?: unknown) => {
            console.log('[Mock Axios] POST', url, data)
            return { data: { success: true } }
        },
        put: async (url: string, data?: unknown) => {
            console.log('[Mock Axios] PUT', url, data)
            return { data: { success: true } }
        },
        delete: async (url: string) => {
            console.log('[Mock Axios] DELETE', url)
            return { data: { success: true } }
        },
        patch: async (url: string, data?: unknown) => {
            console.log('[Mock Axios] PATCH', url, data)
            return { data: { success: true } }
        },
    }
}

// Find plugin definition from module exports
function findPluginDefinition(module: Record<string, unknown>): PluginDefinition | null {
    // Try common export names
    const exportNames = [
        'default',
        'plugin',
        'hexEditorPlugin',
        'myPlugin',
    ]

    for (const name of exportNames) {
        const exported = module[name]
        if (exported && typeof exported === 'object' && 'id' in exported && 'name' in exported) {
            return exported as PluginDefinition
        }
    }

    // Search all exports for something that looks like a plugin definition
    for (const [key, value] of Object.entries(module)) {
        if (value && typeof value === 'object' && 'id' in value && 'name' in value && 'version' in value) {
            console.log(`[Debug] Found plugin definition in export: ${key}`)
            return value as PluginDefinition
        }
    }

    return null
}

// Load and register the plugin
async function loadPlugin(pluginsStore: ReturnType<typeof usePluginsStore>, debugStore: ReturnType<typeof useDebugStore>) {
    // Dynamic import - plugin.js needs window.Vue to be set first
    // @ts-expect-error - Plugin path is configured via Vite alias
    const pluginModule = await import('@plugin/plugin.js')
    const pluginDef = findPluginDefinition(pluginModule as Record<string, unknown>)

    if (!pluginDef) {
        console.error('[Debug] Could not find plugin definition in module exports:', Object.keys(pluginModule))
        throw new Error('No plugin definition found')
    }

    console.log(`[Debug] Loading plugin: ${pluginDef.name} v${pluginDef.version}`)

    // Register plugin
    pluginsStore.registerPlugin(pluginDef)

    // Set plugin info in debug store
    debugStore.setPluginInfo({
        id: pluginDef.id,
        name: pluginDef.name,
        version: pluginDef.version,
    })

    // Register translations
    if (pluginDef.translations) {
        pluginsStore.setPluginTranslations(pluginDef.id, pluginDef.translations)
    }

    // Register file editors
    if (pluginDef.fileEditors) {
        for (const editor of pluginDef.fileEditors) {
            pluginsStore.registerFileEditor(pluginDef.id, editor)
        }
    }

    // Register slots
    if (pluginDef.slots) {
        for (const [slotName, components] of Object.entries(pluginDef.slots)) {
            const comps = Array.isArray(components) ? components : [components]
            for (const comp of comps) {
                pluginsStore.registerSlotComponent(slotName, pluginDef.id, comp.component, {
                    props: comp.props,
                    order: comp.order || 0,
                    label: comp.label,
                    icon: comp.icon,
                    name: comp.name,
                })
            }
        }
    }

    // Register menu items
    if (pluginDef.menuItems) {
        for (const item of pluginDef.menuItems) {
            pluginsStore.registerMenuItem(item.section || 'custom', pluginDef.id, item)
        }
    }

    // Register routes
    if (pluginDef.routes) {
        for (const route of pluginDef.routes) {
            pluginsStore.addPendingRoute(pluginDef.id, route)
        }
    }

    // Call onInit if exists
    if (typeof pluginDef.onInit === 'function') {
        await pluginDef.onInit()
    }

    pluginsStore.setInitialized(true)
    console.log(`[Debug] Plugin loaded successfully`)
}

// Create router
const router = createRouter({
    history: createWebHistory(),
    routes: [
        {
            path: '/',
            name: 'home',
            component: () => import('./views/HomeView.vue'),
        },
        {
            path: '/file-editor',
            name: 'file-editor',
            component: () => import('./views/FileEditorTest.vue'),
        },
        {
            path: '/server-tab',
            name: 'server-tab',
            component: () => import('./views/ServerTabTest.vue'),
        },
        {
            path: '/routes',
            name: 'routes',
            component: () => import('./views/RouteTest.vue'),
        },
    ],
})

// Initialize app
async function init() {
    setupGlobals()

    const app = createApp(App)
    const pinia = createPinia()

    app.use(pinia)
    app.use(router)

    // Get stores
    const debugStore = useDebugStore()
    const pluginsStore = usePluginsStore()

    // Load plugin
    try {
        await loadPlugin(pluginsStore, debugStore)

        // Register plugin routes with router
        pluginsStore.registerRoutes(router)
    } catch (error) {
        console.error('[Debug] Failed to load plugin:', error)
        pluginsStore.addLoadError(String(error))
    }

    app.mount('#app')
}

init()
