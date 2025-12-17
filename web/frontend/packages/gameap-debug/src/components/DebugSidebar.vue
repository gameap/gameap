<template>
    <aside class="w-64 bg-stone-50 dark:bg-stone-900 border-r border-stone-200 dark:border-stone-700 flex flex-col">
        <!-- Header -->
        <div class="p-4 border-b border-stone-200 dark:border-stone-700">
            <h1 class="text-lg font-bold text-stone-800 dark:text-stone-200">
                <i class="fa-solid fa-puzzle-piece mr-2 text-blue-500"></i>
                {{ t('debug.title') }}
            </h1>
            <p v-if="pluginInfo" class="text-sm text-stone-500 dark:text-stone-400 mt-1">
                {{ pluginInfo.name }} v{{ pluginInfo.version }}
            </p>
        </div>

        <!-- Navigation -->
        <nav class="flex-1 p-4 space-y-2">
            <router-link
                v-for="item in navItems"
                :key="item.route"
                :to="item.route"
                class="flex items-center px-3 py-2 rounded-md text-stone-700 dark:text-stone-300 hover:bg-stone-200 dark:hover:bg-stone-800 transition-colors"
                :class="{ 'bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300': isActive(item.route) }"
            >
                <i :class="item.icon" class="w-5 mr-3"></i>
                {{ t(item.label) }}
                <span
                    v-if="item.badge"
                    class="ml-auto text-xs bg-stone-200 dark:bg-stone-700 px-2 py-0.5 rounded-full"
                >
                    {{ item.badge }}
                </span>
            </router-link>
        </nav>

        <!-- Plugin Routes Section -->
        <div v-if="pluginRoutes.length > 0" class="border-t border-stone-200 dark:border-stone-700 p-4">
            <h3 class="text-xs font-semibold text-stone-500 dark:text-stone-400 uppercase mb-2">
                Plugin Routes
            </h3>
            <div class="space-y-1">
                <router-link
                    v-for="route in pluginRoutes"
                    :key="route.name"
                    :to="{ name: route.name }"
                    class="block px-3 py-1.5 text-sm rounded-md text-stone-600 dark:text-stone-400 hover:bg-stone-200 dark:hover:bg-stone-800"
                >
                    {{ route.path }}
                </router-link>
            </div>
        </div>

        <!-- Footer -->
        <div class="p-4 border-t border-stone-200 dark:border-stone-700 text-xs text-stone-500 dark:text-stone-400">
            <div class="flex items-center justify-between">
                <span>GameAP Debug Harness</span>
                <span>v0.1.0</span>
            </div>
        </div>
    </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useDebugStore } from '@/stores/debug'
import { usePluginsStore } from '@/stores/plugins'

const route = useRoute()
const debugStore = useDebugStore()
const pluginsStore = usePluginsStore()

const pluginInfo = computed(() => debugStore.pluginInfo)

const navItems = computed(() => [
    { route: '/', label: 'debug.title', icon: 'fa-solid fa-house' },
    {
        route: '/file-editor',
        label: 'debug.file_editors',
        icon: 'fa-solid fa-file-code',
        badge: pluginsStore.fileEditors.length || null,
    },
    {
        route: '/server-tab',
        label: 'debug.server_tabs',
        icon: 'fa-solid fa-server',
        badge: pluginsStore.getSlotComponents('server-tabs').length || null,
    },
    {
        route: '/routes',
        label: 'debug.routes',
        icon: 'fa-solid fa-route',
        badge: pluginsStore.registeredRoutes.length || null,
    },
])

const pluginRoutes = computed(() => {
    return pluginsStore.registeredRoutes.map(name => ({
        name,
        path: name.replace('plugin.', '/plugins/').replace(/\./g, '/'),
    }))
})

function isActive(path: string): boolean {
    return route.path === path
}

function t(key: string): string {
    const locale = debugStore.locale
    return window.i18n?.[locale]?.[key] || window.i18n?.['en']?.[key] || key
}
</script>
