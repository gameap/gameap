<template>
    <div class="h-full flex">
        <!-- Server Info Panel -->
        <div class="w-72 border-r border-stone-200 dark:border-stone-700 bg-stone-50 dark:bg-stone-900 flex flex-col">
            <div class="p-4 border-b border-stone-200 dark:border-stone-700">
                <h3 class="font-semibold text-stone-800 dark:text-stone-200 mb-3">
                    <i class="fa-solid fa-server mr-2 text-green-500"></i>
                    Server Context
                </h3>

                <!-- Server Selection -->
                <select
                    v-model="debugStore.serverType"
                    class="w-full border border-stone-300 dark:border-stone-600 rounded-md p-2 bg-white dark:bg-stone-800 text-stone-800 dark:text-stone-200 mb-4"
                >
                    <option value="none">{{ t('debug.none') }}</option>
                    <option value="minecraft">{{ t('debug.minecraft') }}</option>
                    <option value="cs">{{ t('debug.cs') }}</option>
                </select>
            </div>

            <!-- Server Details -->
            <div v-if="currentServer" class="flex-1 p-4 overflow-y-auto">
                <div class="space-y-4">
                    <div>
                        <h4 class="text-xs font-semibold text-stone-500 dark:text-stone-400 uppercase mb-2">
                            Server Info
                        </h4>
                        <div class="space-y-2 text-sm">
                            <div class="flex justify-between">
                                <span class="text-stone-500">Name:</span>
                                <span class="font-medium text-stone-800 dark:text-stone-200">{{ currentServer.name }}</span>
                            </div>
                            <div class="flex justify-between">
                                <span class="text-stone-500">ID:</span>
                                <span class="font-mono text-stone-600 dark:text-stone-400">{{ currentServer.id }}</span>
                            </div>
                            <div class="flex justify-between">
                                <span class="text-stone-500">Game:</span>
                                <span class="font-medium text-stone-800 dark:text-stone-200">{{ currentServer.game_id }}</span>
                            </div>
                            <div class="flex justify-between">
                                <span class="text-stone-500">IP:Port:</span>
                                <span class="font-mono text-stone-600 dark:text-stone-400">
                                    {{ currentServer.ip }}:{{ currentServer.port }}
                                </span>
                            </div>
                            <div class="flex justify-between">
                                <span class="text-stone-500">Status:</span>
                                <span
                                    :class="currentServer.process_active ? 'text-green-600' : 'text-red-600'"
                                    class="font-medium"
                                >
                                    {{ currentServer.process_active ? 'Running' : 'Stopped' }}
                                </span>
                            </div>
                        </div>
                    </div>

                    <div>
                        <h4 class="text-xs font-semibold text-stone-500 dark:text-stone-400 uppercase mb-2">
                            Abilities
                        </h4>
                        <div class="flex flex-wrap gap-1">
                            <span
                                v-for="ability in currentAbilities"
                                :key="ability"
                                class="px-2 py-0.5 bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300 text-xs rounded"
                            >
                                {{ ability }}
                            </span>
                        </div>
                    </div>

                    <div>
                        <h4 class="text-xs font-semibold text-stone-500 dark:text-stone-400 uppercase mb-2">
                            Directory
                        </h4>
                        <code class="block text-xs bg-stone-200 dark:bg-stone-800 p-2 rounded overflow-x-auto">
                            {{ currentServer.dir }}
                        </code>
                    </div>
                </div>
            </div>

            <div v-else class="flex-1 flex items-center justify-center p-4 text-stone-500 dark:text-stone-400">
                <div class="text-center">
                    <i class="fa-solid fa-server text-2xl mb-2 opacity-50"></i>
                    <p class="text-sm">No server selected</p>
                </div>
            </div>
        </div>

        <!-- Server Tab Host -->
        <div class="flex-1">
            <ServerTabHost />
        </div>
    </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import ServerTabHost from '@/components/ServerTabHost.vue'
import { useDebugStore } from '@/stores/debug'

const debugStore = useDebugStore()

const currentServer = computed(() => debugStore.currentServer)
const currentAbilities = computed(() => debugStore.currentAbilities)

function t(key: string): string {
    const locale = debugStore.locale
    return window.i18n?.[locale]?.[key] || window.i18n?.['en']?.[key] || key
}
</script>
