<template>
    <div class="p-6">
        <h1 class="text-2xl font-bold text-stone-800 dark:text-stone-200 mb-6">
            <i class="fa-solid fa-puzzle-piece mr-3 text-blue-500"></i>
            Plugin Debug Harness
        </h1>

        <!-- Plugin Info Card -->
        <div v-if="pluginInfo" class="bg-white dark:bg-stone-800 rounded-lg shadow-md p-6 mb-6">
            <h2 class="text-lg font-semibold text-stone-800 dark:text-stone-200 mb-4">
                Loaded Plugin
            </h2>
            <div class="grid grid-cols-2 gap-4 text-sm">
                <div>
                    <span class="text-stone-500 dark:text-stone-400">Name:</span>
                    <span class="ml-2 font-medium text-stone-800 dark:text-stone-200">{{ pluginInfo.name }}</span>
                </div>
                <div>
                    <span class="text-stone-500 dark:text-stone-400">Version:</span>
                    <span class="ml-2 font-medium text-stone-800 dark:text-stone-200">{{ pluginInfo.version }}</span>
                </div>
                <div>
                    <span class="text-stone-500 dark:text-stone-400">ID:</span>
                    <span class="ml-2 font-mono text-xs text-stone-600 dark:text-stone-400">{{ pluginInfo.id }}</span>
                </div>
            </div>
        </div>

        <!-- Stats Grid -->
        <div class="grid grid-cols-3 gap-4 mb-6">
            <router-link
                to="/file-editor"
                class="bg-white dark:bg-stone-800 rounded-lg shadow-md p-4 hover:shadow-lg transition-shadow"
            >
                <div class="flex items-center">
                    <div class="w-12 h-12 rounded-lg bg-purple-100 dark:bg-purple-900 flex items-center justify-center">
                        <i class="fa-solid fa-file-code text-purple-600 dark:text-purple-400 text-xl"></i>
                    </div>
                    <div class="ml-4">
                        <div class="text-2xl font-bold text-stone-800 dark:text-stone-200">
                            {{ fileEditorsCount }}
                        </div>
                        <div class="text-sm text-stone-500 dark:text-stone-400">File Editors</div>
                    </div>
                </div>
            </router-link>

            <router-link
                to="/server-tab"
                class="bg-white dark:bg-stone-800 rounded-lg shadow-md p-4 hover:shadow-lg transition-shadow"
            >
                <div class="flex items-center">
                    <div class="w-12 h-12 rounded-lg bg-green-100 dark:bg-green-900 flex items-center justify-center">
                        <i class="fa-solid fa-server text-green-600 dark:text-green-400 text-xl"></i>
                    </div>
                    <div class="ml-4">
                        <div class="text-2xl font-bold text-stone-800 dark:text-stone-200">
                            {{ serverTabsCount }}
                        </div>
                        <div class="text-sm text-stone-500 dark:text-stone-400">Server Tabs</div>
                    </div>
                </div>
            </router-link>

            <router-link
                to="/routes"
                class="bg-white dark:bg-stone-800 rounded-lg shadow-md p-4 hover:shadow-lg transition-shadow"
            >
                <div class="flex items-center">
                    <div class="w-12 h-12 rounded-lg bg-blue-100 dark:bg-blue-900 flex items-center justify-center">
                        <i class="fa-solid fa-route text-blue-600 dark:text-blue-400 text-xl"></i>
                    </div>
                    <div class="ml-4">
                        <div class="text-2xl font-bold text-stone-800 dark:text-stone-200">
                            {{ routesCount }}
                        </div>
                        <div class="text-sm text-stone-500 dark:text-stone-400">Routes</div>
                    </div>
                </div>
            </router-link>
        </div>

        <!-- Quick Actions -->
        <div class="bg-white dark:bg-stone-800 rounded-lg shadow-md p-6">
            <h2 class="text-lg font-semibold text-stone-800 dark:text-stone-200 mb-4">
                Quick Start
            </h2>
            <div class="space-y-3 text-sm text-stone-600 dark:text-stone-400">
                <div class="flex items-start">
                    <i class="fa-solid fa-circle-check text-green-500 mt-0.5 mr-3"></i>
                    <span>Plugin loaded successfully</span>
                </div>
                <div class="flex items-start">
                    <i class="fa-solid fa-lightbulb text-yellow-500 mt-0.5 mr-3"></i>
                    <span>Use the Debug Panel (bottom-right) to switch between user types, servers, and locales</span>
                </div>
                <div class="flex items-start">
                    <i class="fa-solid fa-info-circle text-blue-500 mt-0.5 mr-3"></i>
                    <span>Click on the stats cards above to test different plugin features</span>
                </div>
            </div>
        </div>

        <!-- Current Context -->
        <div class="mt-6 bg-stone-100 dark:bg-stone-900 rounded-lg p-4">
            <h3 class="text-sm font-semibold text-stone-600 dark:text-stone-400 mb-3">
                Current Context
            </h3>
            <div class="grid grid-cols-2 gap-4 text-sm">
                <div>
                    <span class="text-stone-500">User:</span>
                    <span class="ml-2 font-medium text-stone-800 dark:text-stone-200">
                        {{ currentUser.name }}
                        <span v-if="currentUser.isAdmin" class="text-blue-500">(Admin)</span>
                    </span>
                </div>
                <div>
                    <span class="text-stone-500">Server:</span>
                    <span class="ml-2 font-medium text-stone-800 dark:text-stone-200">
                        {{ currentServer?.name || 'None' }}
                    </span>
                </div>
                <div>
                    <span class="text-stone-500">Locale:</span>
                    <span class="ml-2 font-medium text-stone-800 dark:text-stone-200">
                        {{ debugStore.locale }}
                    </span>
                </div>
                <div v-if="currentServer">
                    <span class="text-stone-500">Game:</span>
                    <span class="ml-2 font-medium text-stone-800 dark:text-stone-200">
                        {{ currentServer.game_id }}
                    </span>
                </div>
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

const pluginInfo = computed(() => debugStore.pluginInfo)
const currentUser = computed(() => debugStore.currentUser)
const currentServer = computed(() => debugStore.currentServer)

const fileEditorsCount = computed(() => pluginsStore.fileEditors.length)
const serverTabsCount = computed(() => pluginsStore.getSlotComponents('server-tabs').length)
const routesCount = computed(() => pluginsStore.registeredRoutes.length)
</script>
