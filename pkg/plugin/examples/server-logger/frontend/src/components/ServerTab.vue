<template>
    <div class="p-4">
        <div class="p-4 border border-stone-200 dark:border-stone-700 rounded-lg bg-white dark:bg-stone-800">
            <h3 class="text-lg font-semibold mb-3 text-stone-900 dark:text-white">
                <i class="fas fa-clipboard-list mr-2"></i>
                {{ trans('server_logger_info') }}
            </h3>

            <div v-if="loading" class="text-stone-600 dark:text-stone-400">
                {{ trans('loading_server_info') }}
            </div>

            <div v-else-if="error" class="text-red-600 dark:text-red-400">
                {{ error }}
            </div>

            <div v-else class="space-y-2">
                <div class="grid grid-cols-2 gap-4">
                    <div>
                        <p class="text-sm text-stone-500 dark:text-stone-400">{{ trans('server_id') }}</p>
                        <p class="font-medium text-stone-900 dark:text-white">{{ serverInfo.id }}</p>
                    </div>
                    <div>
                        <p class="text-sm text-stone-500 dark:text-stone-400">{{ trans('server_name') }}</p>
                        <p class="font-medium text-stone-900 dark:text-white">{{ serverInfo.name }}</p>
                    </div>
                    <div>
                        <p class="text-sm text-stone-500 dark:text-stone-400">{{ trans('ip_address') }}</p>
                        <p class="font-medium text-stone-900 dark:text-white">{{ serverInfo.ip }}</p>
                    </div>
                    <div>
                        <p class="text-sm text-stone-500 dark:text-stone-400">{{ trans('port') }}</p>
                        <p class="font-medium text-stone-900 dark:text-white">{{ serverInfo.port }}</p>
                    </div>
                </div>

                <div class="mt-4 pt-4 border-t border-stone-200 dark:border-stone-700">
                    <p class="text-sm text-stone-500 dark:text-stone-400">{{ trans('requested_by') }}</p>
                    <p class="font-medium text-stone-900 dark:text-white">{{ serverInfo.requestedBy }}</p>
                </div>
            </div>

            <div class="mt-4 pt-4 border-t border-stone-200 dark:border-stone-700">
                <h4 class="text-md font-medium mb-2 text-stone-900 dark:text-white">
                    {{ trans('server_data_props') }}
                </h4>
                <div class="space-y-1 text-sm">
                    <p class="text-stone-700 dark:text-stone-300">
                        <strong>{{ trans('name') }}:</strong> {{ server?.name || trans('na') }}
                    </p>
                    <p class="text-stone-700 dark:text-stone-300">
                        <strong>{{ trans('game') }}:</strong> {{ server?.game_id || trans('na') }}
                    </p>
                    <p class="text-stone-700 dark:text-stone-300">
                        <strong>{{ trans('address') }}:</strong> {{ serverAddress }}
                    </p>
                    <p class="text-stone-700 dark:text-stone-300">
                        <strong>{{ trans('status') }}:</strong>
                        <span :class="statusClass">{{ statusText }}</span>
                    </p>
                </div>
            </div>

            <button
                @click="refreshServerInfo"
                :disabled="loading"
                class="mt-4 px-3 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
            >
                <i class="fas fa-sync-alt mr-1" :class="{ 'fa-spin': loading }"></i>
                {{ trans('refresh') }}
            </button>
        </div>
    </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { usePluginTrans } from '@gameap/plugin-sdk';
import type { ServerTabProps } from '@gameap/plugin-sdk';
import axios from 'axios';

const props = defineProps<ServerTabProps>();
const { trans } = usePluginTrans();

const loading = ref(true);
const error = ref<string | null>(null);
const serverInfo = ref({
    id: 0,
    name: '',
    ip: '',
    port: 0,
    requestedBy: '',
});

const serverAddress = computed(() => {
    if (!props.server) return trans('na');
    return `${props.server.ip}:${props.server.port}`;
});

const statusText = computed(() => {
    if (!props.server) return trans('status_unknown');
    if (props.server.process_active) return trans('status_running');
    if (!props.server.enabled) return trans('status_disabled');
    return trans('status_stopped');
});

const statusClass = computed(() => {
    if (!props.server) return 'text-stone-500';
    if (props.server.process_active) return 'text-green-600 dark:text-green-400';
    if (!props.server.enabled) return 'text-red-600 dark:text-red-400';
    return 'text-yellow-600 dark:text-yellow-400';
});

async function refreshServerInfo() {
    loading.value = true;
    error.value = null;

    try {
        const response = await axios.get(`/api/plugins/${props.pluginId}/servers/${props.serverId}`);
        serverInfo.value = {
            id: response.data.server?.id ?? 0,
            name: response.data.server?.name ?? '',
            ip: response.data.server?.ip ?? '',
            port: response.data.server?.port ?? 0,
            requestedBy: response.data.requested_by ?? '',
        };
    } catch (e) {
        error.value = axios.isAxiosError(e) ? e.message : 'Failed to load server info';
    } finally {
        loading.value = false;
    }
}

onMounted(() => {
    refreshServerInfo();
});
</script>
