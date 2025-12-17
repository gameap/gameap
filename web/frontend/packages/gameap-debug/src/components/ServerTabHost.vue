<template>
    <div class="h-full flex flex-col">
        <!-- Tab Headers -->
        <div v-if="serverTabComponents.length > 0" class="border-b border-stone-200 dark:border-stone-700">
            <div class="flex">
                <button
                    v-for="(tab, idx) in serverTabComponents"
                    :key="idx"
                    @click="activeTab = idx"
                    class="px-4 py-3 text-sm font-medium transition-colors relative"
                    :class="[
                        activeTab === idx
                            ? 'text-blue-600 dark:text-blue-400'
                            : 'text-stone-600 dark:text-stone-400 hover:text-stone-800 dark:hover:text-stone-200'
                    ]"
                >
                    <i v-if="tab.icon" :class="tab.icon" class="mr-2"></i>
                    {{ resolveLabel(tab) }}

                    <!-- Active indicator -->
                    <div
                        v-if="activeTab === idx"
                        class="absolute bottom-0 left-0 right-0 h-0.5 bg-blue-600 dark:bg-blue-400"
                    ></div>
                </button>
            </div>
        </div>

        <!-- Tab Content -->
        <div v-if="serverTabComponents.length > 0 && currentServer" class="flex-1 overflow-auto">
            <component
                :is="serverTabComponents[activeTab].component"
                :server-id="currentServer.id"
                :server="currentServer"
                :plugin-id="serverTabComponents[activeTab].pluginId"
            />
        </div>

        <!-- Empty State -->
        <div
            v-else-if="serverTabComponents.length === 0"
            class="flex-1 flex items-center justify-center text-stone-500 dark:text-stone-400"
        >
            <div class="text-center">
                <i class="fa-solid fa-puzzle-piece text-4xl mb-4 opacity-50"></i>
                <p>{{ t('debug.no_server_tabs') }}</p>
            </div>
        </div>

        <!-- No Server Selected -->
        <div
            v-else-if="!currentServer"
            class="flex-1 flex items-center justify-center text-stone-500 dark:text-stone-400"
        >
            <div class="text-center">
                <i class="fa-solid fa-server text-4xl mb-4 opacity-50"></i>
                <p>Select a server in the Debug Panel</p>
            </div>
        </div>
    </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useDebugStore } from '@/stores/debug'
import { usePluginsStore } from '@/stores/plugins'

const debugStore = useDebugStore()
const pluginsStore = usePluginsStore()

const activeTab = ref(0)

const serverTabComponents = computed(() => {
    return pluginsStore.getSlotComponents('server-tabs')
})

const currentServer = computed(() => debugStore.currentServer)

function resolveLabel(tab: { label: string; pluginId: string }): string {
    if (!tab.label) return 'Tab'
    return pluginsStore.resolvePluginText(tab.pluginId, tab.label)
}

function t(key: string): string {
    const locale = debugStore.locale
    return window.i18n?.[locale]?.[key] || window.i18n?.['en']?.[key] || key
}
</script>
