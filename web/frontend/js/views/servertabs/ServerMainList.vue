<script setup>
    import {h, ref, onMounted, computed} from 'vue'
    import {trans} from "@/i18n/i18n";
    import store from "@/legacy/store";
    import {useAuthStore} from "@/store/auth";
    import {useServerFiltersStore} from "@/store/serverFilters";

    import GButton from "@/components/GButton.vue";
    import Loading from "@/components/Loading.vue";
    import GameIcon from "@/components/GameIcon.vue";

    import ServerControlButton  from "./ServerControlButton.vue";

    import {errorNotification} from "@/parts/dialogs";

    // Installed statuses
    const NOT_INSTALLED        = 0;
    const INSTALLED            = 1;
    const INSTALLATION_PROCESS = 2;

    const authStore = useAuthStore();
    const filtersStore = useServerFiltersStore();

    const createColumns = () => {
        return [
            {
                title: trans('servers.name'),
                key: "name",
                render(row) {
                  return h("div", {class: 'flex items-center'}, [
                    h(GameIcon, {game: row.game.code, class: "mr-2"}),
                    h("span", {class: ''}, row.name)
                  ])
                },
            },
            {
                title: trans('servers.ip_port'),
                render(row) {
                    return row.server_ip + ":" + row.server_port;
                }
            },
            {
                title: trans('servers.status'),
                key: "status",
                render(row) {
                    if (row.blocked) {
                        return h('span', {class: "badge-stone"}, trans('servers.blocked'));
                    }

                    if (!row.enabled) {
                        return h('span', {class: "badge-stone"}, trans('servers.disabled'));
                    }

                    if (!row.installed) {
                        return h('span', {class: "badge-stone"}, trans('servers.not_installed'));
                    }

                    if (row.installed === INSTALLATION_PROCESS) {
                        return h('span', {class: "badge-orange"}, trans('servers.installation'));
                    }

                    if (row.online) {
                        return h(
                            "span",
                            {
                                class: "badge-green",
                            },
                            trans('servers.online'),
                        );
                    }

                    return h(
                        "span",
                        {
                            class: "badge-red",
                        },
                        trans('servers.offline'),
                    );
                }
            },
            {
                title: trans('servers.commands'),
                render(row) {
                    if (!row.enabled || row.blocked) {
                        return [];
                    }

                    if (row.installed === NOT_INSTALLED && canUpdate(row.id)) {
                        return h(ServerControlButton,
                            {
                                "command": "install",
                                "button-color": "orange",
                                "button-size": "small",
                                "icon": "fa fa-download",
                                "text": trans('servers.install'),
                                "server-id": row.id,
                            });
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
                                        "icon": "fa fa-stop",
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
                                        "icon": "fa fa-play",
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
                                    "icon": "fa fa-redo",
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
                              [
                                h('span', {"class": "hidden lg:inline"}, trans('servers.control')),
                                " ",
                                h('span', {"class": "fa fa-angle-double-right"}),
                              ])
                      );
                    }

                    return buttons;
                }
            },
        ];
    };

    const columns = ref(createColumns());
    const pagination = {
        pageSize: 20,
    };
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

    onMounted(() => {
      store.dispatch('servers/fetchServers').finally(() => {
        loading.value = false;
      });

      if (!authStore.isAdmin) {
        authStore.fetchServersAbilities().catch((error) =>{
          errorNotification(error)
        })
      }
    });

    const data = computed(() => {
        return store.state.servers.serversList.filter((server) => {
            let skip = false;

            if (
                selectedGame.value !== null &&
                selectedGame.value !== "" &&
                selectedGame.value.length > 0
            ) {
                skip = !selectedGame.value.includes(server.game.code)
            }

            if (
                !skip &&
                selectedIP.value !== null &&
                selectedIP.value !== "" &&
                selectedIP.value.length > 0
            ) {
                skip = !selectedIP.value.includes(server.server_ip)
            }

            return !skip
        });
    });

    const renderGameLabel = (option) => {
        return [
            h(GameIcon, {game: option.value, class: 'mr-2'}),
            option.label,
        ]
    }

    const games = computed(() => {
        const map = new Map;
        for (const idx in store.state.servers.serversList) {
            map.set(
                store.state.servers.serversList[idx].game.code,
                store.state.servers.serversList[idx].game.name,
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
        for (const idx in store.state.servers.serversList) {
            set.add(store.state.servers.serversList[idx].server_ip)
        }

        var options = [];
        for (const el of Array.from(set).sort()) {
            options.push({label: el, value: el});
        }

        return options;
    });

    function handleUpdateFilters() {
        // Filters are automatically updated via computed properties
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
                            <i class="fa fa-eraser"></i><span class="hidden lg:inline">&nbsp;{{ trans('main.clear') }}</span>
                        </n-button>
                    </div>
                </div>
            </n-collapse-item>
        </n-collapse>
    </div>

    <n-data-table
        ref="tableRef"
        :bordered="false"
        :single-line="true"
        :columns="columns"
        :data="data"
        :loading="loading"
        :pagination="pagination"
    >
        <template #loading>
          <Loading />
        </template>
        <template #empty>
            <n-empty :description="trans('servers.empty_list')">
            </n-empty>
        </template>
    </n-data-table>
</template>
