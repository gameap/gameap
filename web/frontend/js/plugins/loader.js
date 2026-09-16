import axios from '../config/axios'
import * as Vue from 'vue'
import * as VueRouter from 'vue-router'
import * as Pinia from 'pinia'
import * as NaiveUI from 'naive-ui/es/index.mjs'
import * as gameapUI from '@gameap/ui'
import { usePluginsStore } from '../store/plugins'

// Expose Vue and related libraries globally for pre-compiled plugin components
// When plugins are built with external: ['vue', 'vue-router', 'pinia', 'axios', '@gameap/ui'],
// they expect these to be available globally
window.Vue = Vue
window.VueRouter = VueRouter
window.Pinia = Pinia
window.axios = axios
window.NaiveUI = NaiveUI
// The debug harness historically exposed naive-ui as `naive`
window.naive = NaiveUI
window.gameapUI = gameapUI

// Keep backwards compatibility with render function plugins
window.__gameap_vue_h = Vue.h

async function loadPluginStyles() {
    try {
        const response = await axios.get('/plugins.css', {
            responseType: 'text',
            headers: {
                'Accept': 'text/css'
            }
        })

        if (response.data && response.data.trim() !== '') {
            const existingStyle = document.getElementById('gameap-plugin-styles')
            if (existingStyle) {
                existingStyle.remove()
            }

            const style = document.createElement('style')
            style.id = 'gameap-plugin-styles'
            style.textContent = response.data
            document.head.appendChild(style)
        }
    } catch (error) {
        if (error.response?.status !== 404) {
            console.error('Failed to load plugin styles:', error)
        }
    }
}

// Plugins are loaded once per page. app.js loads them for a restored session,
// and SsoView loads them again after redeeming a ticket — which also runs
// while that restored session is still active. The store appends every menu
// item, slot component and editor it is given, so a second pass rendered each
// of them twice and re-ran every onInit. The bundle is the same for every
// account, so later callers share the first load. A load that registered
// nothing is not remembered, which lets the next caller retry it.
let pluginsLoad = null

export function loadPlugins(router) {
    if (!pluginsLoad) {
        pluginsLoad = fetchAndRegisterPlugins(router).then((registered) => {
            if (!registered) {
                pluginsLoad = null
            }
        })
    }

    return pluginsLoad
}

// Resolves to false when the bundle could not be fetched or evaluated, which
// always happens before anything is registered.
async function fetchAndRegisterPlugins(router) {
    const pluginsStore = usePluginsStore()

    pluginsStore.setLoading(true)

    await loadPluginStyles()

    try {
        const response = await axios.get('/plugins.js', {
            responseType: 'text',
            headers: {
                'Accept': 'application/javascript'
            }
        })

        // Bundles built before the SDK stripped bare side-effect imports of
        // externalized modules would break the blob-module import below.
        const moduleText = response.data
            .replace(/^\s*import\s*["'](?:@gameap\/ui|naive-ui)["'];?\s*$/gm, '')

        if (!moduleText || moduleText.trim() === '') {
            console.log('No plugins to load')

            return true
        }

        const blob = new Blob([moduleText], { type: 'application/javascript' })
        const moduleUrl = URL.createObjectURL(blob)

        try {
            const module = await import(/* @vite-ignore */ moduleUrl)

            for (const [exportName, pluginDef] of Object.entries(module)) {
                if (exportName === 'default' && typeof pluginDef !== 'object') {
                    continue
                }

                const def = exportName === 'default' ? pluginDef : pluginDef

                if (!def || typeof def !== 'object') {
                    continue
                }

                try {
                    await registerPluginDefinition(def, pluginsStore)
                } catch (error) {
                    console.error(`Failed to register plugin ${exportName}:`, error)
                    pluginsStore.addLoadError(`${exportName}: ${error.message}`)
                }
            }
        } finally {
            URL.revokeObjectURL(moduleUrl)
        }

        pluginsStore.registerRoutes(router)

        return true
    } catch (error) {
        if (error.response?.status === 404) {
            console.log('No plugins endpoint available')

            return true
        }

        if (error.__CANCEL__) {
            return false
        }

        console.error('Plugin loader error:', error)
        pluginsStore.addLoadError(error.message)

        return false
    } finally {
        pluginsStore.setLoading(false)
        pluginsStore.setInitialized(true)
    }
}

async function registerPluginDefinition(pluginDef, store) {
    if (!pluginDef.id || !pluginDef.name || !pluginDef.version) {
        throw new Error('Plugin missing required fields (id, name, version)')
    }

    const pluginId = store.registerPlugin(pluginDef)

    if (pluginDef.translations) {
        store.setPluginTranslations(pluginId, pluginDef.translations)
    }

    if (pluginDef.routes) {
        for (const route of pluginDef.routes) {
            store.addPendingRoute(pluginId, route)
        }
    }

    if (pluginDef.menuItems) {
        for (const item of pluginDef.menuItems) {
            const menuItem = {
                ...item,
                route: item.route ? normalizeRoute(item.route, pluginId) : null
            }
            store.registerMenuItem(item.section || 'custom', pluginId, menuItem)
        }
    }

    if (pluginDef.slots) {
        for (const [slotName, slotComponents] of Object.entries(pluginDef.slots)) {
            const components = Array.isArray(slotComponents) ? slotComponents : [slotComponents]
            for (const slotComp of components) {
                store.registerSlotComponent(slotName, pluginId, slotComp.component, {
                    props: slotComp.props,
                    order: slotComp.order || 0,
                    label: slotComp.label,
                    icon: slotComp.icon,
                    name: slotComp.name,
                    checkPermission: slotComp.checkPermission,
                    checkGame: slotComp.checkGame,
                })
            }
        }
    }

    if (pluginDef.homeButtons) {
        const buttons = Array.isArray(pluginDef.homeButtons) ? pluginDef.homeButtons : [pluginDef.homeButtons]
        for (const btn of buttons) {
            store.registerSlotComponent('home-buttons', pluginId, btn.component || null, {
                order: btn.order || 0,
                label: btn.name,
                icon: btn.icon || 'puzzle-piece',
                name: btn.name,
                props: {
                    route: btn.route ? normalizeRoute(btn.route, pluginId) : { name: `plugin.${pluginId}.index` }
                }
            })
        }
    }

    if (pluginDef.fileEditors) {
        for (const editor of pluginDef.fileEditors) {
            store.registerFileEditor(pluginId, editor)
        }
    }

    if (typeof pluginDef.onInit === 'function') {
        try {
            await pluginDef.onInit()
        } catch (error) {
            console.error(`Plugin ${pluginDef.id} onInit failed:`, error)
        }
    }
}

function normalizeRoute(route, pluginId) {
    if (typeof route === 'string') {
        return { path: `/plugins/${pluginId}${route}` }
    }

    if (route.name) {
        return { name: `plugin.${pluginId}.${route.name}` }
    }

    if (route.path) {
        return { path: `/plugins/${pluginId}${route.path}` }
    }

    return route
}

/**
 * Resolves translation reference in format @:key
 * Returns { isTranslationKey: true, key: 'key' } if it's a translation reference
 * Returns { isTranslationKey: false, value: 'text' } if it's plain text
 */
export function parseTranslationRef(text) {
    if (typeof text === 'string' && text.startsWith('@:')) {
        return { isTranslationKey: true, key: text.slice(2) }
    }
    return { isTranslationKey: false, value: text }
}

/**
 * Resolves a text value that might be a translation reference (@:key)
 */
export function resolvePluginText(text, translations, locale = 'en') {
    const parsed = parseTranslationRef(text)
    if (parsed.isTranslationKey) {
        const langTrans = translations?.[locale] || translations?.['en'] || {}
        return langTrans[parsed.key] ?? parsed.key
    }
    return parsed.value
}
