<template>
    <div id="server-status-component">
        <div class="flex flex-wrap ">
            <div class="md:w-1/6 pr-4 pl-4">
                <GStatusBadge v-if="status === 'online'" status="success" :text="trans('servers.online')" />
                <GStatusBadge v-else-if="status === 'offline'" status="error" :text="trans('servers.offline')" />
                <GStatusBadge v-else status="waiting" text="-" />
            </div>

            <div v-if="showHostname" class="md:w-1/3 pr-4 pl-4">
                <div v-if="useJoinLink" class="inline">
                    <a :href="queryInfo.joinlink">{{ queryInfo.hostname }}</a>
                </div>

                <div v-else>{{ queryInfo.hostname }}</div>
            </div>

            <div v-if="showPlayersNum" class="md:w-1/4 pr-4 pl-4">
              {{ trans('servers.query_players') }}: {{ queryInfo.players }}
            </div>

            <div v-if="showMap" class="md:w-1/4 pr-4 pl-4">
              {{ trans('servers.query_map') }}: {{ queryInfo.map }}
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
