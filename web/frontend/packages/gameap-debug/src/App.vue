<template>
    <div class="h-screen flex bg-stone-100 dark:bg-stone-950 text-stone-900 dark:text-stone-100">
        <!-- Sidebar -->
        <DebugSidebar />

        <!-- Main Content -->
        <main class="flex-1 flex flex-col overflow-hidden">
            <router-view v-slot="{ Component }">
                <ContextProvider>
                    <component :is="Component" class="flex-1 overflow-auto" />
                </ContextProvider>
            </router-view>
        </main>

        <!-- Debug Panel -->
        <DebugPanel />
    </div>
</template>

<script setup lang="ts">
import { defineComponent, h } from 'vue'
import DebugSidebar from '@/components/DebugSidebar.vue'
import DebugPanel from '@/components/DebugPanel.vue'
import { providePluginContext } from '@/context/provider'

// Context provider wrapper component
const ContextProvider = defineComponent({
    name: 'ContextProvider',
    setup(_, { slots }) {
        providePluginContext()
        return () => slots.default?.()
    },
})
</script>

<!-- Styles are in src/styles/main.css -->
