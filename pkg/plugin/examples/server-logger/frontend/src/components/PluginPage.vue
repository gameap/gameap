<template>
    <div class="p-4">
        <h1 class="text-xl font-bold mb-4 text-stone-900 dark:text-white">
            <i class="fas fa-clipboard-list mr-2"></i>
            {{ trans('title') }}
        </h1>
        <p class="text-stone-600 dark:text-stone-400 mb-4">
            {{ trans('description') }}
        </p>

        <div class="grid gap-4 md:grid-cols-2">
            <div class="p-4 border border-stone-200 dark:border-stone-700 rounded-lg bg-white dark:bg-stone-800">
                <h2 class="font-semibold mb-2 text-stone-900 dark:text-white">
                    <i class="fas fa-heartbeat mr-1"></i>
                    {{ trans('plugin_status') }}
                </h2>

                <div v-if="statusLoading" class="text-stone-600 dark:text-stone-400">
                    {{ trans('loading_status') }}
                </div>

                <div v-else-if="statusError" class="text-red-600 dark:text-red-400">
                    {{ statusError }}
                </div>

                <div v-else class="space-y-2">
                    <p class="text-stone-600 dark:text-stone-400">
                        <strong>{{ trans('status') }}:</strong>
                        <span class="ml-1 text-green-600 dark:text-green-400">{{ status.status }}</span>
                    </p>
                    <p class="text-stone-600 dark:text-stone-400">
                        <strong>{{ trans('plugin') }}:</strong> {{ status.plugin }}
                    </p>
                    <p class="text-stone-600 dark:text-stone-400">
                        <strong>{{ trans('version') }}:</strong> {{ status.version }}
                    </p>
                </div>
            </div>

            <div class="p-4 border border-stone-200 dark:border-stone-700 rounded-lg bg-white dark:bg-stone-800">
                <h2 class="font-semibold mb-2 text-stone-900 dark:text-white">
                    <i class="fas fa-chart-bar mr-1"></i>
                    {{ trans('statistics') }}
                </h2>

                <div v-if="statsLoading" class="text-stone-600 dark:text-stone-400">
                    {{ trans('loading_stats') }}
                </div>

                <div v-else-if="statsError" class="text-red-600 dark:text-red-400">
                    {{ statsError }}
                </div>

                <div v-else class="space-y-2">
                    <p class="text-stone-600 dark:text-stone-400">
                        <strong>{{ trans('events_processed') }}:</strong>
                        <span class="ml-1 text-2xl font-bold text-blue-600 dark:text-blue-400">
                            {{ stats.eventsProcessed }}
                        </span>
                    </p>
                    <p class="text-stone-600 dark:text-stone-400">
                        <strong>{{ trans('requested_by') }}:</strong> {{ stats.requestedBy }}
                    </p>
                </div>
            </div>
        </div>

        <div class="mt-4 p-4 border border-stone-200 dark:border-stone-700 rounded-lg bg-white dark:bg-stone-800">
            <h2 class="font-semibold mb-2 text-stone-900 dark:text-white">
                <i class="fas fa-user mr-1"></i>
                {{ trans('current_user') }}
            </h2>
            <p class="text-stone-600 dark:text-stone-400">
                {{ trans('logged_in_as') }}: <strong>{{ user.login }}</strong>
            </p>
            <p class="text-stone-600 dark:text-stone-400">
                {{ trans('roles') }}: {{ user.roles?.join(', ') || trans('none') }}
            </p>
            <p class="text-stone-600 dark:text-stone-400">
                {{ trans('admin') }}: {{ user.isAdmin ? trans('yes') : trans('no') }}
            </p>
        </div>

        <div class="mt-4 p-4 border border-stone-200 dark:border-stone-700 rounded-lg bg-white dark:bg-stone-800">
            <h2 class="font-semibold mb-2 text-stone-900 dark:text-white">
                <i class="fas fa-info-circle mr-1"></i>
                {{ trans('about') }}
            </h2>
            <p class="text-stone-600 dark:text-stone-400">
                {{ trans('about_text') }}
            </p>
            <ul class="mt-2 list-disc list-inside text-stone-600 dark:text-stone-400">
                <li>{{ trans('feature_events') }}</li>
                <li>{{ trans('feature_game_info') }}</li>
                <li>{{ trans('feature_http_api') }}</li>
                <li>{{ trans('feature_frontend') }}</li>
            </ul>
        </div>

        <div class="mt-4 flex gap-2">
            <button
                @click="refresh"
                :disabled="statusLoading || statsLoading"
                class="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
            >
                <i class="fas fa-sync-alt mr-1" :class="{ 'fa-spin': statusLoading || statsLoading }"></i>
                {{ trans('refresh_all') }}
            </button>
        </div>
    </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { useCurrentUser, usePluginTrans } from '@gameap/plugin-sdk';
import axios from 'axios';

const user = useCurrentUser();
const { trans } = usePluginTrans();
const pluginId = 'fwgfo26jzwnm4';

const statusLoading = ref(true);
const statusError = ref<string | null>(null);
const status = ref({
    status: '',
    plugin: '',
    version: '',
});

const statsLoading = ref(true);
const statsError = ref<string | null>(null);
const stats = ref({
    eventsProcessed: 0,
    requestedBy: '',
});

async function loadStatus() {
    statusLoading.value = true;
    statusError.value = null;

    try {
        const response = await axios.get(`/api/plugins/${pluginId}/status`);
        status.value = {
            status: response.data.status ?? 'unknown',
            plugin: response.data.plugin ?? '',
            version: response.data.version ?? '',
        };
    } catch (e) {
        statusError.value = axios.isAxiosError(e) ? e.message : 'Failed to load status';
    } finally {
        statusLoading.value = false;
    }
}

async function loadStats() {
    statsLoading.value = true;
    statsError.value = null;

    try {
        const response = await axios.get(`/api/plugins/${pluginId}/stats`);
        stats.value = {
            eventsProcessed: response.data.events_processed ?? 0,
            requestedBy: response.data.requested_by ?? '',
        };
    } catch (e) {
        statsError.value = axios.isAxiosError(e) ? e.message : 'Failed to load stats';
    } finally {
        statsLoading.value = false;
    }
}

function refresh() {
    loadStatus();
    loadStats();
}

onMounted(() => {
    refresh();
});
</script>
