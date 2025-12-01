<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <InactiveServer v-if="!loading && !isServerEnabled" :server="server"></InactiveServer>
  <n-tabs v-else type="line" class="flex justify-between" :class="(!isServerEnabled) ? 'hidden': ''" animated>
    <n-tab-pane name="control">
      <template #tab>
        <i class="fas fa-play mr-1"></i>
        {{ trans('servers.control') }}
      </template>

      <div class="md:flex md:flex-wrap mt-2" v-show="serverQueryOnline">
        <div class="md:w-full">
          <n-card
              class="mb-3"
          >
            <Loading v-if="loading"></Loading>
            <ServerStatus v-if="!loading" ref="serverStatusRef" :server-id="serverId"></ServerStatus>
          </n-card>
        </div>
      </div>

      <div class="md:flex mt-2">
        <div class="md:w-1/2 md:pr-8">
          <n-card
              :title="trans('servers.commands')"
              class="mb-3"
              header-class="g-card-header"
              :segmented="{
                          content: true,
                          footer: 'soft'
                        }"
          >
            <Loading v-if="loading"></Loading>
            <div v-if="!loading" id="serverControl">
              <ServerControlButton
                  command="start"
                  v-if="serverStore.canStart && !serverOnline"
                  :server-id="serverId"
                  button="m-1"
                  button-color="green"
                  icon="fas fa-play"
                  :text="trans('servers.start')"
              ></ServerControlButton>

              <ServerControlButton
                  command="stop"
                  v-if="serverStore.canStop && serverOnline"
                  :server-id="serverId"
                  button="m-1"
                  button-color="red"
                  icon="fas fa-stop"
                  :text="trans('servers.stop')"
              ></ServerControlButton>

              <ServerControlButton
                  command="restart"
                  v-if="serverStore.canRestart"
                  :server-id="serverId"
                  button="m-1"
                  button-color="orange"
                  icon="fas fa-redo"
                  :text="trans('servers.restart')"
              ></ServerControlButton>

              <ServerControlButton
                  command="update"
                  v-if="serverStore.canUpdate"
                  :server-id="serverId"
                  button="m-1"
                  button-color="black"
                  icon="fas fa-sync"
                  :text="trans('servers.update')"
              ></ServerControlButton>

              <ServerControlButton
                  command="reinstall"
                  v-if="serverStore.canUpdate"
                  :server-id="serverId"
                  button="m-1"
                  button-color="black"
                  icon="fas fa-reply-all"
                  :text="trans('servers.reinstall')"
              ></ServerControlButton>
            </div>
          </n-card>
        </div>

        <div class="md:w-1/2">
          <n-card
              :title="trans('servers.process_status')"
              class="mb-3"
              header-class="g-card-header"
              :segmented="{
                          content: true,
                          footer: 'soft'
                        }"
          >
            <Loading v-if="loading"></Loading>
            <ul v-if="!loading" class="flex flex-col pl-0 mb-0">
              <li v-if="serverOnline" class="relative block py-3 px-6 -mb-px">
                {{ trans('servers.status') }}: <span class="badge-green">{{ trans('servers.active') }}</span>
              </li>
              <li v-else class="relative block py-3 px-6 -mb-px">
                {{ trans('servers.status') }}: <span class="badge-red">{{ trans('servers.inactive') }}</span>
              </li>

              <li class="relative block py-3 px-6 -mb-px">
                {{ trans('servers.last_check') }}: {{ (new Date(server.last_process_check)).toLocaleString() }}
              </li>
            </ul>
          </n-card>
        </div>
      </div>

      <div class="md:flex flex-wrap mt-2" v-if="serverStore.canReadConsole">
        <div class="md:w-full">
          <Loading v-if="loading"></Loading>
          <ServerConsole
              v-if="!loading"
              :console-hostname="server?.name"
              :server-id="serverId"
              :server-active="server?.online"
              :send-command-available="serverStore.canSendConsole"
          >
          </ServerConsole>
        </div>
      </div>

    </n-tab-pane>

    <n-tab-pane name="rcon" v-if="rconTabPossible">
      <template #tab>
        <i class="fas fa-user-astronaut mr-1"></i>
        RCON
      </template>

      <div class="flex flex-wrap mt-2">
        <div class="md:w-full">
          <div :class="'md:grid ' + (rconSupportedFeatures.playersManage ? 'md:grid-cols-2' : 'md:grid-cols-1')">
            <div v-if="rconSupportedFeatures.playersManage" class="pr-8">
              <n-card
                  :title="trans('rcon.players_manage')"
                  class="mb-3"
                  header-class="g-card-header"
                  :segmented="{
                      content: true,
                      footer: 'soft'
                    }">
                <Loading v-if="loading"></Loading>
                <rcon-players :server-id="serverId" v-if="!loading" />
              </n-card>
            </div>

            <div>
              <n-card
                  :title="trans('rcon.console')"
                  class="mb-3"
                  header-class="g-card-header"
                  :segmented="{
                      content: true,
                      footer: 'soft'
                    }">
                <Loading v-if="loading"></Loading>
                <rcon-console :server-id="serverId" v-if="!loading" />
              </n-card>
            </div>

          </div>
        </div>
      </div>
    </n-tab-pane>

    <n-tab-pane name="files" v-if="serverStore.canManageFiles">
      <template #tab>
        <i class="fa fa-folder-open mr-1"></i>
        {{ trans('servers.files') }}
      </template>

      <div class="flex flex-wrap mt-2 h-[calc(100vh-14rem)]">
        <div class="md:w-full h-full">
          <div
              class="flex flex-col min-w-0 rounded break-words border bg-white
              dark:bg-stone-800 border-1 border-stone-300 dark:border-stone-700 h-full"
          >
            <file-manager
                :settings="{
                    'lang': pageLanguage(),
                    'baseUrl': '/api/file-manager/'+$route.params.id,
                    'headers':{
                        'X-Requested-With': 'XMLHttpRequest'
                    }
                }"
            />
          </div>
        </div>
      </div>
    </n-tab-pane>

    <n-tab-pane name="schedules" v-if="serverStore.canManageTasks">
      <template #tab>
        <i class="far fa-calendar-alt mr-1"></i>
        {{ trans('servers.task_scheduler') }}
      </template>

      <ServerTasks
          :server-id="serverId"
          :privileges="privileges"
      ></ServerTasks>
    </n-tab-pane>

    <n-tab-pane name="settings" v-if="serverStore.canManageSettings">
      <template #tab>
        <i class="fa fa-cogs mr-1"></i>
        {{ trans('servers.settings') }}
      </template>

      <div class="md:flex md:flex-wrap mt-2">
        <div class="md:w-full">
          <n-card class="mb-3">
            <div>
              <Loading v-if="loading" class="absolute inset-0 flex justify-center items-center z-10"></Loading>
              <ServerSettings :class="loading ? 'opacity-20' : ''"></ServerSettings>
            </div>
          </n-card>
        </div>
      </div>
    </n-tab-pane>

    <template #suffix v-if="isAdmin">
      <div class="order-last ml-auto text-red-500 hover:text-red-600">
        <router-link :to="{name: 'admin.servers.edit', params: {id: serverId}}">
          <i class="fa fa-hammer mr-1"></i>
          {{ trans('servers.admin') }}
        </router-link>
      </div>
    </template>

  </n-tabs>
</template>

<script setup>
import {computed, h, defineAsyncComponent, onMounted, ref} from "vue";
import {useRoute} from "vue-router";
import {storeToRefs} from "pinia";
import ServerControlButton from "./servertabs/ServerControlButton.vue";

const ServerStatus = defineAsyncComponent(() =>
    import('./servertabs/ServerStatus.vue' /* webpackChunkName: "components/server" */)
)

const ServerTasks = defineAsyncComponent(() =>
    import('./servertabs/ServerTasks.vue' /* webpackChunkName: "components/server" */)
)

const ServerConsole = defineAsyncComponent(() =>
    import('./servertabs/ServerConsole.vue' /* webpackChunkName: "components/server" */)
)

const ServerSettings = defineAsyncComponent(() =>
    import('./servertabs/ServerSettings.vue' /* webpackChunkName: "components/server" */)
)

const RconPlayers = defineAsyncComponent(() =>
    import('../components/rcon/RconPlayers.vue' /* webpackChunkName: "components/rcon" */)
)

const RconConsole = defineAsyncComponent(() =>
    import('../components/rcon/RconConsole.vue' /* webpackChunkName: "components/rcon" */)
)

import GBreadcrumbs from "../components/GBreadcrumbs.vue";
import Loading from "../components/Loading.vue";

import {useServerStore} from "@/store/server"
import {useServerRconStore} from "@/store/serverRcon"
import {useAuthStore} from "@/store/auth";
import {trans, pageLanguage} from "@/i18n/i18n";
import GameIcon from "../components/GameIcon.vue";
import InactiveServer from "./InactiveServer.vue";

const route = useRoute()
const serverStore = useServerStore()
const serverRconStore = useServerRconStore()
const authStore = useAuthStore()

const {
  serverId,
  server,
} = storeToRefs(serverStore)

const {
  rconSupportedFeatures,
} = storeToRefs(serverRconStore)

const loading = computed(() => {
  return serverStore.loading
})

onMounted(() => {
  serverStore.setServerId(Number(route.params.id))

  serverStore.fetchServer().then(() => {
    if (server.value) {
      document.title = server.value.name
    }

    isServerEnabled.value = server.value?.enabled
        && !server.value?.blocked
        && server.value?.installed === 1

    if (isServerEnabled.value) {
      serverRconStore.fetchRconSupportedFeatures()
    }
  })
  serverStore.fetchAbilities()
});

const isServerEnabled = ref(true)
const serverStatusRef = ref(null)

const privileges = computed(() => {
  return {
    'start': serverStore.canStart,
    'stop': serverStore.canStop,
    'restart': serverStore.canRestart,
    'update': serverStore.canUpdate,
  }
})

const serverOnline = computed(() => {
  return Boolean(server.value?.online)
})

const serverQueryOnline = computed(() => {
  return serverStatusRef.value?.status === 'online'
})

const rconTabPossible = computed(() => {
  return (rconSupportedFeatures.value.rcon || rconSupportedFeatures.value.playersManage) &&
      serverRconStore.canUseRcon &&
      serverOnline.value
})

const breadcrumbs = computed(() => {
  const bc = [
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
    {'route':{name: 'servers'}, 'text':trans('servers.game_servers')},
  ]

  if (server.value?.name) {
    bc.push({
      render: () => [
        h(GameIcon, {game: server.value.game?.code ?? '', class: 'mr-2 align-middle'}),
        h('span', {game: server.value.game?.code ?? '', class: 'align-middle'}, server.value.name),
      ]
    })
  }

  return bc
})

const isAdmin = computed(() => {
  return authStore.isAdmin
})

</script>