<script setup>
    import { Loading, GIcon, GDataTable, GEmpty, GGameIcon } from "@gameap/ui";
    import {h, ref, reactive, onMounted, onUnmounted, computed, watch} from 'vue'
    import {storeToRefs} from 'pinia'
    import {trans} from "@/i18n/i18n";
    import {useAuthStore} from "@/store/auth";
    import {useServerFiltersStore} from "@/store/serverFilters";
    import {useServerListStore} from "@/store/serverList";
    import {usePluginsStore} from "@/store/plugins";
    import {filterSlotComponents} from "@/plugins/permissions";

    import GButton from "@/components/GButton.vue";

    import ServerControlButton  from "./ServerControlButton.vue";

    import {errorNotification} from "@/parts/dialogs";
    import {NTooltip} from "naive-ui";

    // Installed statuses
    const NOT_INSTALLED        = 0;
    const INSTALLED            = 1;
    const INSTALLATION_PROCESS = 2;

    const authStore = useAuthStore();
    const filtersStore = useServerFiltersStore();
    const serverListStore = useServerListStore();
    const pluginsStore = usePluginsStore();
    const { servers: serversList, currentPage, perPage, total, lastPage } = storeToRefs(serverListStore);

    const isSmallScreen = ref(window.innerWidth < 768);

    const handleResize = () => {
        isSmallScreen.value = window.innerWidth < 768;
    };

    const columns = computed(() => {
        const cols = [
            {
                title: trans('servers.name'),
                key: "name",
                render(row) {
                    return h("div", {class: 'flex items-center'}, [
                        h(GGameIcon, {game: row.game.code, class: "mr-2"}),
                        h("div", {}, [
                            h("span", {}, row.name),
                            isSmallScreen.value
                                ? h("small", {class: "block text-stone-500 dark:text-stone-400"},
                                    row.server_ip + ":" + row.server_port)
                                : null
                        ].filter(Boolean))
                    ])
                },
            },
        ];

        if (!isSmallScreen.value) {
            cols.push({
                title: trans('servers.ip_port'),
                render(row) {
                    return row.server_ip + ":" + row.server_port;
                }
            });
        }

        cols.push({
            title: trans('servers.status'),
            key: "status",
            render(row) {
                let badgeClass;
                let circleClass;
                let statusText;

                if (row.blocked) {
                    badgeClass = "badge-stone";
                    circleClass = "badge-circle-stone";
                    statusText = trans('servers.blocked');
                } else if (!row.enabled) {
                    badgeClass = "badge-stone";
                    circleClass = "badge-circle-stone";
                    statusText = trans('servers.disabled');
                } else if (!row.installed) {
                    badgeClass = "badge-stone";
                    circleClass = "badge-circle-stone";
                    statusText = trans('servers.not_installed');
                } else if (row.installed === INSTALLATION_PROCESS) {
                    badgeClass = "badge-orange";
                    circleClass = "badge-circle-orange";
                    statusText = trans('servers.installation');
                } else if (row.online) {
                    badgeClass = "badge-green";
                    circleClass = "badge-circle-green";
                    statusText = trans('servers.online');
                } else {
                    badgeClass = "badge-red";
                    circleClass = "badge-circle-red";
                    statusText = trans('servers.offline');
                }

                if (isSmallScreen.value) {
                    return h(NTooltip, {
                        trigger: 'hover',
                    }, {
                        trigger: () => h('span', {class: circleClass}),
                        default: () => statusText
                    });
                }

                return h('span', {class: badgeClass}, statusText);
            }
        });

        cols.push({
            title: trans('servers.commands'),
            render(row) {
                if (!row.enabled || row.blocked) {
                    return [];
                }

                if (row.installed === NOT_INSTALLED && canUpdate(row.id)) {
                    return [
                        h(ServerControlButton,
                            {
                                "command": "install",
                                "button-color": "orange",
                                "button-size": "small",
                                "icon": "download",
                                "text": trans('servers.install'),
                                "server-id": row.id,
                            }),
                        " ",
                        ...pluginActions(row),
                    ];
                }

                let buttons = [];

                if (row.installed === INSTALLED) {
                    if (row.online && canStop(row.id)) {
                        buttons.push(
                            h(ServerControlButton,
                                {
                                    "command": "stop",
                                    "button-color": "red",
                                    "button-size": "small",
                                    "icon": "stop",
                                    "text": trans('servers.stop'),
                                    "server-id": row.id,
                                }),
                            " ",
                        );
                    }

                    if (!row.online && canStart(row.id)) {
                        buttons.push(
                            h(ServerControlButton,
                                {
                                    "command": "start",
                                    "button-color": "green",
                                    "button-size": "small",
                                    "icon": "play",
                                    "text": trans('servers.start'),
                                    "server-id": row.id,
                                }),
                            " "
                        );
                    }

                    if (canRestart(row.id)) {
                        buttons.push(
                            h(ServerControlButton,
                                {
                                    "command": "restart",
                                    "button-color": "orange",
                                    "button-size": "small",
                                    "icon": "restart",
                                    "text": trans('servers.restart'),
                                    "server-id": row.id,
                                }),
                            " ",
                        );
                    }
                }

                if (canManage(row.id)) {
                    buttons.push(
                        h(GButton,
                            {
                                color: "black",
                                size: "small",
                                route: "/servers/" + row.id,
                            },
                            { default: () => [
                                h('span', {"class": "hidden lg:inline"}, trans('servers.control')),
                                " ",
                                h(GIcon, {name: "chevron-double-right"}),
                            ]})
                    );
                }

                buttons.push(...pluginActions(row));

                return buttons;
            }
        });

        return cols;
    });

    const pagination = reactive({
        page: 1,
        pageSize: 30,
        pageCount: 1,
        itemCount: 0,
    });

    const loading = ref(true);
    const tableRef = ref(null);

    const selectedGame = computed({
        get: () => filtersStore.selectedGame,
        set: (value) => filtersStore.setGameFilter(value)
    });

    const selectedIP = computed({
        get: () => filtersStore.selectedIP,
        set: (value) => filtersStore.setIPFilter(value)
    });

    const fetchServers = () => {
        loading.value = true;

        return serverListStore.fetchServersByFilter({
            page: pagination.page,
            perPage: pagination.pageSize,
            gameIds: (selectedGame.value && selectedGame.value.length > 0) ? selectedGame.value : null,
        })
            .then(() => {
                pagination.pageCount = lastPage.value;
                pagination.itemCount = total.value;
                pagination.page = currentPage.value;
                pagination.pageSize = perPage.value;
            })
            .catch((error) => {
                errorNotification(error);
            })
            .finally(() => {
                loading.value = false;
            });
    };

    const handlePageChange = (page) => {
        pagination.page = page;
        fetchServers();
    };

    watch(selectedGame, () => {
        pagination.page = 1;
        fetchServers();
    });

    onMounted(() => {
        window.addEventListener('resize', handleResize);

        fetchServers();

        if (!authStore.isAdmin) {
            authStore.fetchServersAbilities().catch((error) =>{
                errorNotification(error)
            })
        }
    });

    onUnmounted(() => {
        window.removeEventListener('resize', handleResize);
    });

    // IP filter remains client-side (applied on the current page only).
    const data = computed(() => {
        return serversList.value.filter((server) => {
            return !(selectedIP.value !== null &&
                selectedIP.value !== "" &&
                selectedIP.value.length > 0 &&
                !selectedIP.value.includes(server.server_ip));
        });
    });

    const renderGameLabel = (option) => {
        return [
            h(GGameIcon, {game: option.value, class: 'mr-2'}),
            option.label,
        ]
    }

    const games = computed(() => {
        const map = new Map;
        for (const idx in serversList.value) {
            map.set(
                serversList.value[idx].game.code,
                serversList.value[idx].game.name,
            )
        }

        let sorted = [];
        map.forEach((name, code) => {
          sorted.push([code, name])
        });

        sorted.sort((a, b) => {
          return a[1].localeCompare(b[1])
        });

        let result = [];
        sorted.forEach((value) => {
          result.push({
            value: value[0],
            label: value[1],
          })
        });

        return result
    });

    const gamesOptions = computed(() => {
        var options = [];

        for (const el of games.value) {
            options.push({label: el.label, value: el.value});
        }
        return options;
    });

    const gamesIPOptions = computed(() => {
        const set = new Set;
        for (const idx in serversList.value) {
            set.add(serversList.value[idx].server_ip)
        }

        var options = [];
        for (const el of Array.from(set).sort()) {
            options.push({label: el, value: el});
        }

        return options;
    });

    function handleUpdateFilters() {
        // Filters are automatically updated via computed properties.
        // Server-side filters (game) trigger a refetch via the watcher.
    }

    function clearFilters() {
        filtersStore.clearFilters();
    }

    function isFiltersSet() {
        return filtersStore.hasFilters
    }

    function canManage(serverId) {
      return authStore.canServerAbility(serverId, 'game-server-common')
    }

    function canStart(serverId) {
        return authStore.canServerAbility(serverId, 'game-server-start');
    }

    function canStop(serverId) {
        return authStore.canServerAbility(serverId, 'game-server-stop');
    }

    function canRestart(serverId) {
        return authStore.canServerAbility(serverId, 'game-server-restart');
    }

    function canUpdate(serverId) {
        return authStore.canServerAbility(serverId, 'game-server-update');
    }

    // The list keeps abilities per server in the auth store rather than as a plain
    // map, so the plugin permission check reads them through a lazy lookup.
    function serverAbilities(serverId) {
        return new Proxy({}, {
            get: (_target, ability) => authStore.canServerAbility(serverId, ability) === true,
        });
    }

    function pluginActions(row) {
        if (!pluginsStore.isInitialized) {
            return [];
        }

        return filterSlotComponents(
            pluginsStore.getSlotComponents('servers-list-actions'),
            {abilities: serverAbilities(row.id), game: row.game},
        ).map(item => h(item.component, {
            ...item.props,
            serverId: row.id,
            server: row,
            pluginId: item.pluginId,
        }));
    }

</script>

<template>
    <div class="flex flex-wrap  mb-4">
        <n-collapse>
            <n-collapse-item :title="trans('servers.filters')" name="filters">
                <div class="flex flex-wrap ">
                    <div class="md:w-1/4 pr-4 pl-4">
                        <n-select
                            v-model:value="selectedGame"
                            :options="gamesOptions"
                            :render-label="renderGameLabel"
                            multiple
                            :placeholder="trans('servers.select_game_filter_placeholder')"
                            @update:value="handleUpdateFilters"
                        >
                        </n-select>
                    </div>

                    <div class="md:w-1/4 pr-4 pl-4">
                        <n-select
                            v-model:value="selectedIP"
                            :options="gamesIPOptions"
                            multiple
                            :placeholder="trans('servers.select_ip_filter_placeholder')"
                            @update:value="handleUpdateFilters"
                        >
                        </n-select>
                    </div>

                    <div class="md:w-1/4 pr-4 pl-4">
                        <n-button @click="clearFilters" type="error" :disabled="!isFiltersSet()" ghost>
                            <GIcon name="eraser" /><span class="hidden lg:inline">&nbsp;{{ trans('main.clear') }}</span>
                        </n-button>
                    </div>
                </div>
            </n-collapse-item>
        </n-collapse>
    </div>

    <GDataTable
        remote
        ref="tableRef"
        :columns="columns"
        :data="data"
        :loading="loading"
        :pagination="pagination"
        :scroll-x="isSmallScreen ? 520 : 800"
        @update:page="handlePageChange"
    >
        <template #loading>
          <Loading />
        </template>
        <template #empty>
            <GEmpty :description="trans('servers.empty_list')" />
        </template>
    </GDataTable>
</template>
