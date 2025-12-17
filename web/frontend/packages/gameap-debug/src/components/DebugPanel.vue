<template>
    <div class="fixed bottom-4 right-4 z-50">
        <!-- Collapsed button -->
        <button
            v-if="debugStore.debugPanelCollapsed"
            @click="debugStore.toggleDebugPanel"
            class="w-12 h-12 rounded-full bg-blue-600 text-white shadow-lg hover:bg-blue-700 flex items-center justify-center"
        >
            <i class="fa-solid fa-bug"></i>
        </button>

        <!-- Expanded panel -->
        <div
            v-else
            class="w-80 bg-white dark:bg-stone-800 border border-stone-200 dark:border-stone-700 rounded-lg shadow-xl"
        >
            <!-- Header -->
            <div
                class="p-3 bg-stone-100 dark:bg-stone-900 rounded-t-lg flex justify-between items-center cursor-pointer"
                @click="debugStore.toggleDebugPanel"
            >
                <span class="font-semibold text-stone-800 dark:text-stone-200">
                    <i class="fa-solid fa-bug mr-2"></i>
                    Debug Controls
                </span>
                <button class="text-stone-500 hover:text-stone-700 dark:hover:text-stone-300">
                    <i class="fa-solid fa-minus"></i>
                </button>
            </div>

            <!-- Controls -->
            <div class="p-4 space-y-4">
                <!-- User Type -->
                <div>
                    <label class="block text-sm font-medium text-stone-700 dark:text-stone-300 mb-1">
                        {{ t('debug.user_type') }}
                    </label>
                    <select
                        v-model="debugStore.userType"
                        class="w-full border border-stone-300 dark:border-stone-600 rounded-md p-2 bg-white dark:bg-stone-700 text-stone-800 dark:text-stone-200"
                    >
                        <option value="admin">{{ t('debug.admin') }}</option>
                        <option value="user">{{ t('debug.user') }}</option>
                        <option value="guest">{{ t('debug.guest') }}</option>
                    </select>
                </div>

                <!-- Server -->
                <div>
                    <label class="block text-sm font-medium text-stone-700 dark:text-stone-300 mb-1">
                        {{ t('debug.server') }}
                    </label>
                    <select
                        v-model="debugStore.serverType"
                        class="w-full border border-stone-300 dark:border-stone-600 rounded-md p-2 bg-white dark:bg-stone-700 text-stone-800 dark:text-stone-200"
                    >
                        <option value="none">{{ t('debug.none') }}</option>
                        <option value="minecraft">{{ t('debug.minecraft') }}</option>
                        <option value="cs">{{ t('debug.cs') }}</option>
                    </select>
                </div>

                <!-- Locale -->
                <div>
                    <label class="block text-sm font-medium text-stone-700 dark:text-stone-300 mb-1">
                        {{ t('debug.locale') }}
                    </label>
                    <select
                        v-model="debugStore.locale"
                        class="w-full border border-stone-300 dark:border-stone-600 rounded-md p-2 bg-white dark:bg-stone-700 text-stone-800 dark:text-stone-200"
                    >
                        <option value="en">English</option>
                        <option value="ru">Русский</option>
                    </select>
                </div>

                <!-- Dark Mode Toggle -->
                <div class="flex items-center justify-between">
                    <span class="text-sm font-medium text-stone-700 dark:text-stone-300">
                        Dark Mode
                    </span>
                    <button
                        @click="debugStore.toggleDarkMode"
                        class="p-2 rounded-md hover:bg-stone-100 dark:hover:bg-stone-700"
                    >
                        <i :class="debugStore.isDarkMode ? 'fa-solid fa-sun text-yellow-500' : 'fa-solid fa-moon text-stone-500'"></i>
                    </button>
                </div>

                <!-- Plugin Info -->
                <div v-if="debugStore.pluginInfo" class="pt-3 border-t border-stone-200 dark:border-stone-700">
                    <div class="text-sm text-stone-500 dark:text-stone-400">
                        <div class="flex justify-between">
                            <span>Plugin:</span>
                            <span class="font-medium text-stone-700 dark:text-stone-300">
                                {{ debugStore.pluginInfo.name }}
                            </span>
                        </div>
                        <div class="flex justify-between">
                            <span>Version:</span>
                            <span class="font-medium text-stone-700 dark:text-stone-300">
                                {{ debugStore.pluginInfo.version }}
                            </span>
                        </div>
                        <div class="flex justify-between">
                            <span>ID:</span>
                            <span class="font-mono text-xs text-stone-600 dark:text-stone-400">
                                {{ debugStore.pluginInfo.id }}
                            </span>
                        </div>
                    </div>
                </div>

                <!-- Current Context Info -->
                <div class="pt-3 border-t border-stone-200 dark:border-stone-700 text-xs text-stone-500 dark:text-stone-400">
                    <div v-if="debugStore.currentServer">
                        <span class="font-medium">Server:</span>
                        {{ debugStore.currentServer.name }} ({{ debugStore.currentServer.game_id }})
                    </div>
                    <div>
                        <span class="font-medium">User:</span>
                        {{ debugStore.currentUser.name }}
                        <span v-if="debugStore.currentUser.isAdmin" class="text-blue-500">(admin)</span>
                    </div>
                </div>
            </div>
        </div>
    </div>
</template>

<script setup lang="ts">
import { useDebugStore } from '@/stores/debug'

const debugStore = useDebugStore()

function t(key: string): string {
    const locale = debugStore.locale
    return window.i18n?.[locale]?.[key] || window.i18n?.['en']?.[key] || key
}
</script>
