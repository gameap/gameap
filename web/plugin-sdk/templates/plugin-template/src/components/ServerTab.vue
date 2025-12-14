<template>
    <div class="p-4">
        <div class="p-4 border border-stone-200 dark:border-stone-700 rounded-lg bg-white dark:bg-stone-800">
            <h3 class="text-lg font-semibold mb-3 text-stone-900 dark:text-white">
                My Plugin Tab
            </h3>

            <div class="space-y-2">
                <p class="text-stone-700 dark:text-stone-300">
                    <strong>Server ID:</strong> {{ serverId }}
                </p>
                <p class="text-stone-700 dark:text-stone-300">
                    <strong>Server Name:</strong> {{ server?.name || 'Loading...' }}
                </p>
                <p class="text-stone-700 dark:text-stone-300">
                    <strong>Game:</strong> {{ server?.game_id || 'N/A' }}
                </p>
                <p class="text-stone-700 dark:text-stone-300">
                    <strong>Address:</strong> {{ serverAddress }}
                </p>
                <p class="text-stone-700 dark:text-stone-300">
                    <strong>Status:</strong>
                    <span :class="statusClass">{{ statusText }}</span>
                </p>
            </div>

            <p class="mt-4 text-stone-600 dark:text-stone-400">
                This tab was added by your plugin. It demonstrates access to server data.
            </p>
        </div>
    </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { ServerTabProps } from '@gameap/plugin-sdk';

const props = defineProps<ServerTabProps>();

const serverAddress = computed(() => {
    if (!props.server) return 'N/A';
    return `${props.server.ip}:${props.server.port}`;
});

const statusText = computed(() => {
    if (!props.server) return 'Unknown';
    if (props.server.process_active) return 'Running';
    if (!props.server.enabled) return 'Disabled';
    return 'Stopped';
});

const statusClass = computed(() => {
    if (!props.server) return 'text-stone-500';
    if (props.server.process_active) return 'text-green-600 dark:text-green-400';
    if (!props.server.enabled) return 'text-red-600 dark:text-red-400';
    return 'text-yellow-600 dark:text-yellow-400';
});
</script>
