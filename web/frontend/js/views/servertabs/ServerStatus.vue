<template>
    <div id="server-status-component">
        <div class="flex flex-col md:flex-row md:items-center gap-3 md:gap-0
                    md:divide-x divide-stone-200 dark:divide-stone-700">
            <div v-if="showHostname" class="md:flex-1 md:px-4 min-w-0">
                <a v-if="useJoinLink" :href="queryInfo.joinlink" class="block text-sm truncate text-sky-500 hover:underline">{{ queryInfo.hostname }}</a>
                <span v-else class="block text-sm truncate">{{ queryInfo.hostname }}</span>
            </div>

            <div v-if="showPlayersNum" class="flex items-center gap-2 md:flex-1 md:px-4">
                <span class="text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400 shrink-0">
                    {{ trans('servers.query_players') }}
                </span>
                <span class="text-sm font-mono tabular-nums">{{ queryInfo.players }}</span>
            </div>

            <div v-if="showMap" class="flex items-center gap-2 md:flex-1 md:px-4 min-w-0">
                <span class="text-xs uppercase tracking-wide text-stone-500 dark:text-stone-400 shrink-0">
                    {{ trans('servers.query_map') }}
                </span>
                <span class="text-sm truncate min-w-0">{{ queryInfo.map }}</span>
            </div>
        </div>
    </div>
</template>

<script setup>
    import { ref, computed, onMounted } from 'vue';
    import axios from '@/config/axios';

    const props = defineProps({
        serverId: Number
    });

    const queryInfo = ref(null);

    const status = computed(() => {
        if (!queryInfo.value?.status) {
            return 'unknown';
        }
        return queryInfo.value.status === 'online' ? 'online' : 'offline';
    });

    const showHostname = computed(() => 'hostname' in (queryInfo.value ?? {}));
    const showPlayersNum = computed(() => 'players' in (queryInfo.value ?? {}));
    const showMap = computed(() => 'map' in (queryInfo.value ?? {}));
    const useJoinLink = computed(() =>
        queryInfo.value?.joinlink && queryInfo.value.joinlink.length > 0
    );

    onMounted(() => {
        axios.get('/api/servers/' + props.serverId + '/query')
            .then(response => (queryInfo.value = response.data));
    });

    defineExpose({
        status,
    });
</script>
