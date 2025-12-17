import { defineStore } from 'pinia'
import { ref, reactive, computed, h, type Component } from 'vue'
import type { Router } from 'vue-router'

const SUPPORTED_API_VERSIONS = ['1.0']

interface PluginInfo {
    id: string
    name: string
    version: string
    apiVersion: string
    enabled: boolean
    loadError: string | null
    loadedAt: Date
}

interface SlotComponent {
    pluginId: string
    component: Component
    props: Record<string, unknown>
    order: number
    label: string
    icon: string | null
    name: string
    checkPermission: (() => boolean) | null
}

interface MenuItem {
    pluginId: string
    icon: string
    text: string
    route: { name: string } | { path: string }
    order: number
    section: string
}

interface FileEditor {
    id: string
    name: string
    component: Component
    match: EditorMatchRules
    contentType?: 'text' | 'binary'
    readOnly?: boolean
    icon?: string
    _compiledRegexp?: RegExp | null
}

interface EditorMatchRules {
    allFiles?: boolean
    fileName?: string
    pathContains?: string
    fullPath?: string
    extensions?: string[]
    fileNameRegexp?: string
    gameCode?: string
    gameName?: string
}

interface RegisteredEditor {
    pluginId: string
    editor: FileEditor
}

interface FileInfo {
    fileName: string
    filePath: string
    extension: string
}

interface ServerContext {
    gameCode?: string
    gameName?: string
}

interface PendingRoute {
    pluginId: string
    path: string
    name: string
    component: Component
    meta: Record<string, unknown>
    children?: PendingRoute[]
}

export const usePluginsStore = defineStore('plugins', () => {
    // State
    const plugins = ref<Map<string, PluginInfo>>(new Map())
    const slots = reactive<Record<string, SlotComponent[]>>({
        'server-tabs': [],
        'dashboard-widgets': [],
        'sidebar-sections': [],
        'admin-pages': [],
        'home-buttons': [],
    })
    const pendingRoutes = ref<PendingRoute[]>([])
    const registeredRoutes = ref<string[]>([])
    const menuItems = reactive<Record<string, MenuItem[]>>({
        servers: [],
        admin: [],
        custom: [],
    })
    const loading = ref(false)
    const initialized = ref(false)
    const loadErrors = ref<string[]>([])
    const pluginTranslations = ref<Map<string, Record<string, Record<string, string>>>>(new Map())
    const fileEditors = ref<RegisteredEditor[]>([])

    // Getters
    const isLoading = computed(() => loading.value)
    const isInitialized = computed(() => initialized.value)
    const hasErrors = computed(() => loadErrors.value.length > 0)
    const enabledPlugins = computed(() => {
        return Array.from(plugins.value.values()).filter(p => p.enabled)
    })

    function getSlotComponents(slotName: string): SlotComponent[] {
        return slots[slotName] || []
    }

    function getMenuItems(section: string): MenuItem[] {
        return menuItems[section] || []
    }

    function getPlugin(id: string): PluginInfo | undefined {
        return plugins.value.get(id)
    }

    // Actions
    function checkApiVersion(version: string): boolean {
        return SUPPORTED_API_VERSIONS.includes(version)
    }

    function registerPlugin(pluginDef: {
        id: string
        name: string
        version: string
        apiVersion?: string
    }): string {
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
            loadedAt: new Date(),
        })

        return pluginDef.id
    }

    function registerSlotComponent(
        slotName: string,
        pluginId: string,
        component: Component,
        options: Partial<Omit<SlotComponent, 'pluginId' | 'component'>> = {}
    ) {
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
            name: options.name || '',
            checkPermission: options.checkPermission || null,
        })

        slots[slotName].sort((a, b) => a.order - b.order)
    }

    function registerMenuItem(
        section: string,
        pluginId: string,
        item: Omit<MenuItem, 'pluginId'>
    ) {
        const targetSection = menuItems[section] ? section : 'custom'

        menuItems[targetSection].push({
            pluginId,
            icon: item.icon || 'fas fa-puzzle-piece',
            text: item.text,
            route: item.route,
            order: item.order || 100,
            section: item.section,
        })

        menuItems[targetSection].sort((a, b) => a.order - b.order)
    }

    function addPendingRoute(
        pluginId: string,
        route: {
            path: string
            name?: string
            component: Component
            meta?: Record<string, unknown>
            children?: Array<{
                path: string
                name?: string
                component: Component
                meta?: Record<string, unknown>
            }>
        }
    ) {
        pendingRoutes.value.push({
            pluginId,
            ...route,
            path: `/plugins/${pluginId}${route.path}`,
            name: `plugin.${pluginId}.${route.name || 'index'}`,
            meta: {
                ...route.meta,
                pluginId,
                requiresAuth: route.meta?.requiresAuth !== false,
            },
        })
    }

    function registerRoutes(router: Router) {
        for (const route of pendingRoutes.value) {
            try {
                router.addRoute({
                    path: route.path,
                    name: route.name,
                    component: route.component,
                    meta: route.meta,
                })
                registeredRoutes.value.push(route.name)
            } catch (error) {
                console.error(`Failed to register route ${route.path}:`, error)
            }
        }
        pendingRoutes.value = []
    }

    function setLoading(value: boolean) {
        loading.value = value
    }

    function setInitialized(value: boolean) {
        initialized.value = value
    }

    function addLoadError(error: string) {
        loadErrors.value.push(error)
    }

    function setPluginTranslations(
        pluginId: string,
        translations: Record<string, Record<string, string>>
    ) {
        pluginTranslations.value.set(pluginId, translations)
    }

    function getPluginTranslations(
        pluginId: string
    ): Record<string, Record<string, string>> | undefined {
        return pluginTranslations.value.get(pluginId)
    }

    function resolvePluginText(pluginId: string, text: string): string {
        if (typeof text !== 'string' || !text.startsWith('@:')) {
            return text
        }
        const key = text.slice(2)
        const translations = pluginTranslations.value.get(pluginId)
        const locale = window.gameapLang || 'en'
        const langTrans = translations?.[locale] || translations?.['en'] || {}
        return langTrans[key] ?? key
    }

    function registerFileEditor(pluginId: string, editor: FileEditor) {
        fileEditors.value.push({
            pluginId,
            editor: {
                ...editor,
                _compiledRegexp: editor.match.fileNameRegexp
                    ? new RegExp(editor.match.fileNameRegexp)
                    : null,
            },
        })
    }

    function calculateMatchScore(
        editor: FileEditor,
        fileInfo: FileInfo,
        serverContext?: ServerContext | null
    ): number {
        const { match } = editor
        const { fileName, filePath, extension } = fileInfo
        const { gameCode, gameName } = serverContext || {}

        let score = 0
        let hasMatch = false

        if (match.gameCode && match.gameCode !== gameCode) {
            return 0
        }
        if (match.gameName && match.gameName !== gameName) {
            return 0
        }

        if (match.gameCode && match.gameCode === gameCode) {
            score += 50
            hasMatch = true
        }
        if (match.gameName && match.gameName === gameName) {
            score += 40
            hasMatch = true
        }

        // allFiles: true matches all files with base score of 1
        if (match.allFiles) {
            return score + 1
        }

        if (match.fullPath) {
            const normalizedFullPath = match.fullPath.replace(/^\//, '')
            const normalizedFilePath = filePath.replace(/^\//, '')
            if (normalizedFilePath === normalizedFullPath) {
                score += 1000
                hasMatch = true
            } else {
                return 0
            }
        }

        if (match.pathContains) {
            if (filePath.includes(match.pathContains)) {
                score += 500
                hasMatch = true
            } else {
                return 0
            }
        }

        if (match.fileName) {
            if (fileName === match.fileName) {
                score += 200
                hasMatch = true
            } else {
                return 0
            }
        }

        if (editor._compiledRegexp) {
            if (editor._compiledRegexp.test(fileName)) {
                score += 100
                hasMatch = true
            } else {
                return 0
            }
        }

        if (match.extensions && match.extensions.length > 0) {
            const ext = extension?.toLowerCase()
            if (ext && match.extensions.map(e => e.toLowerCase()).includes(ext)) {
                score += 10
                hasMatch = true
            } else if (!hasMatch) {
                return 0
            }
        }

        if (!hasMatch) {
            return 0
        }

        return score
    }

    function getMatchingEditors(
        fileInfo: FileInfo,
        serverContext: ServerContext | null = null
    ): Array<RegisteredEditor & { score: number; isDefault: boolean }> {
        const matches: Array<RegisteredEditor & { score: number; isDefault: boolean }> = []

        for (const { pluginId, editor } of fileEditors.value) {
            const score = calculateMatchScore(editor, fileInfo, serverContext)
            if (score > 0) {
                matches.push({
                    pluginId,
                    editor,
                    score,
                    isDefault: false,
                })
            }
        }

        matches.sort((a, b) => b.score - a.score)

        if (matches.length > 0) {
            matches[0].isDefault = true
        }

        return matches
    }

    function getDefaultEditor(
        fileInfo: FileInfo,
        serverContext: ServerContext | null = null
    ): (RegisteredEditor & { score: number; isDefault: boolean }) | null {
        const matches = getMatchingEditors(fileInfo, serverContext)
        return matches.length > 0 ? matches[0] : null
    }

    function unregisterPlugin(pluginId: string) {
        plugins.value.delete(pluginId)

        for (const slotName of Object.keys(slots)) {
            slots[slotName] = slots[slotName].filter(c => c.pluginId !== pluginId)
        }

        for (const section of Object.keys(menuItems)) {
            menuItems[section] = menuItems[section].filter(i => i.pluginId !== pluginId)
        }

        fileEditors.value = fileEditors.value.filter(e => e.pluginId !== pluginId)
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
        fileEditors,

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
        getMatchingEditors,
        getDefaultEditor,

        // Actions
        checkApiVersion,
        registerPlugin,
        registerSlotComponent,
        registerMenuItem,
        registerFileEditor,
        addPendingRoute,
        registerRoutes,
        setLoading,
        setInitialized,
        addLoadError,
        setPluginTranslations,
        unregisterPlugin,
    }
})
