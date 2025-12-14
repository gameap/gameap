<template>
    <div class="w-full mt-5 p-3 border border-stone-200 bg-stone-50 rounded-lg sm:p-4 dark:bg-stone-800 dark:border-stone-700">
        <strong class="text-stone-900 dark:text-white">
            <i class="fas fa-clipboard-list mr-1"></i>
            {{ trans('dashboard_widget') }}
        </strong>

        <div v-if="loading" class="mt-2 text-stone-600 dark:text-stone-400">
            {{ trans('loading_stats') }}
        </div>

        <div v-else-if="error" class="mt-2 text-red-600 dark:text-red-400">
            {{ error }}
        </div>

        <div v-else class="mt-2 space-y-1">
            <p class="text-stone-600 dark:text-stone-400">
                <span class="font-semibold">{{ trans('events_processed') }}:</span>
                <span class="ml-1 text-blue-600 dark:text-blue-400">{{ stats.eventsProcessed }}</span>
            </p>
            <p v-if="stats.requestedBy" class="text-sm text-stone-500 dark:text-stone-500">
                {{ trans('requested_by') }}: {{ stats.requestedBy }}
            </p>
        </div>

        <button
            @click="refreshStats"
            :disabled="loading"
            class="mt-3 px-3 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
        >
            <i class="fas fa-sync-alt mr-1" :class="{ 'fa-spin': loading }"></i>
            {{ trans('refresh') }}
        </button>
    </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { usePluginTrans } from '@gameap/plugin-sdk';
import type { DashboardWidgetProps } from '@gameap/plugin-sdk';
import axios from 'axios';

const props = defineProps<DashboardWidgetProps>();
const { trans } = usePluginTrans();

const loading = ref(true);
const error = ref<string | null>(null);
const stats = ref({
    eventsProcessed: 0,
    requestedBy: '',
});

async function refreshStats() {
    loading.value = true;
    error.value = null;

    try {
        const response = await axios.get(`/api/plugins/${props.pluginId}/stats`);
        stats.value = {
            eventsProcessed: response.data.events_processed ?? 0,
            requestedBy: response.data.requested_by ?? '',
        };
    } catch (e) {
        error.value = axios.isAxiosError(e) ? e.message : 'Failed to load stats';
    } finally {
        loading.value = false;
    }
}

onMounted(() => {
    refreshStats();
});
</script>
