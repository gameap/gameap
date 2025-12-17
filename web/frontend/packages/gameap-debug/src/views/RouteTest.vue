<template>
    <div class="h-full flex flex-col">
        <!-- Routes Header -->
        <div class="p-4 border-b border-stone-200 dark:border-stone-700 bg-stone-50 dark:bg-stone-900">
            <h2 class="text-lg font-semibold text-stone-800 dark:text-stone-200">
                <i class="fa-solid fa-route mr-2 text-blue-500"></i>
                Plugin Routes
            </h2>
            <p class="text-sm text-stone-500 dark:text-stone-400 mt-1">
                {{ pluginsStore.registeredRoutes.length }} route(s) registered
            </p>
        </div>

        <!-- Routes List -->
        <div v-if="pluginsStore.registeredRoutes.length > 0" class="flex-1 overflow-auto p-4">
            <div class="space-y-2">
                <div
                    v-for="routeName in pluginsStore.registeredRoutes"
                    :key="routeName"
                    class="bg-white dark:bg-stone-800 rounded-lg border border-stone-200 dark:border-stone-700 p-4"
                >
                    <div class="flex items-center justify-between">
                        <div>
                            <div class="font-medium text-stone-800 dark:text-stone-200">
                                {{ routeName }}
                            </div>
                            <div class="text-sm text-stone-500 dark:text-stone-400 font-mono mt-1">
                                {{ getRoutePath(routeName) }}
                            </div>
                        </div>
                        <router-link
                            :to="{ name: routeName }"
                            class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-md text-sm flex items-center"
                        >
                            <i class="fa-solid fa-arrow-right mr-2"></i>
                            Open
                        </router-link>
                    </div>
                </div>
            </div>

            <!-- Menu Items Section -->
            <div v-if="hasMenuItems" class="mt-6">
                <h3 class="text-sm font-semibold text-stone-600 dark:text-stone-400 uppercase mb-3">
                    Menu Items
                </h3>
                <div class="space-y-2">
                    <div
                        v-for="(items, section) in allMenuItems"
                        :key="section"
                    >
                        <div v-if="items.length > 0" class="mb-2">
                            <div class="text-xs text-stone-500 uppercase mb-1">{{ section }}</div>
                            <div
                                v-for="item in items"
                                :key="item.text"
                                class="bg-stone-100 dark:bg-stone-700 rounded p-2 flex items-center"
                            >
                                <i :class="item.icon" class="w-5 mr-2 text-stone-500"></i>
                                <span class="text-sm text-stone-700 dark:text-stone-300">
                                    {{ pluginsStore.resolvePluginText(item.pluginId, item.text) }}
                                </span>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <!-- Empty State -->
        <div
            v-else
            class="flex-1 flex items-center justify-center text-stone-500 dark:text-stone-400"
        >
            <div class="text-center">
                <i class="fa-solid fa-route text-4xl mb-4 opacity-50"></i>
                <p>{{ t('debug.no_routes') }}</p>
                <p class="text-sm mt-2">
                    The plugin hasn't registered any routes.
                </p>
            </div>
        </div>
    </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useDebugStore } from '@/stores/debug'
import { usePluginsStore } from '@/stores/plugins'

const debugStore = useDebugStore()
const pluginsStore = usePluginsStore()

const allMenuItems = computed(() => ({
    servers: pluginsStore.getMenuItems('servers'),
    admin: pluginsStore.getMenuItems('admin'),
    custom: pluginsStore.getMenuItems('custom'),
}))

const hasMenuItems = computed(() => {
    return Object.values(allMenuItems.value).some(items => items.length > 0)
})

function getRoutePath(routeName: string): string {
    // Convert plugin.pluginId.routeName to /plugins/pluginId/routeName
    const parts = routeName.split('.')
    if (parts[0] === 'plugin' && parts.length >= 2) {
        const pluginId = parts[1]
        const rest = parts.slice(2).join('/')
        return `/plugins/${pluginId}/${rest || ''}`
    }
    return routeName
}

function t(key: string): string {
    const locale = debugStore.locale
    return window.i18n?.[locale]?.[key] || window.i18n?.['en']?.[key] || key
}
</script>
