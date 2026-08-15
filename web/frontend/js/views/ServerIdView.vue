<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <InactiveServer v-if="!loading && !isServerEnabled" :server="server"></InactiveServer>
  <n-tabs
    v-else
    v-model:value="activeTab"
    type="line"
    :class="(!isServerEnabled) ? 'hidden': ''"
    animated
    display-directive="show:lazy"
    @update:value="onTabChange"
  >
    <n-tab-pane name="control">
      <template #tab>
        <GIcon name="play" class="mr-1" />
        {{ trans('servers.control') }}
      </template>

      <div class="mt-2">
        <n-card size="small" class="mb-3">
          <Loading v-if="loading"></Loading>
          <div v-else class="flex flex-wrap items-center gap-y-2">
            <div id="serverControl" class="flex flex-wrap items-center">
              <ServerControlButton
                  command="start"
                  v-if="serverStore.canStart && !serverOnline"
                  :server-id="serverId"
                  button="m-1"
                  button-color="green"
                  icon="play"
                  :text="trans('servers.start')"
              ></ServerControlButton>

              <ServerControlButton
                  command="stop"
                  v-if="serverStore.canStop && serverOnline"
                  :server-id="serverId"
                  button="m-1"
                  button-color="red"
                  icon="stop"
                  :text="trans('servers.stop')"
              ></ServerControlButton>

              <ServerControlButton
                  command="restart"
                  v-if="serverStore.canRestart"
                  :server-id="serverId"
                  button="m-1"
                  button-color="orange"
                  icon="restart"
                  :text="trans('servers.restart')"
              ></ServerControlButton>

              <ServerControlButton
                  command="update"
                  v-if="serverStore.canUpdate"
                  :server-id="serverId"
                  button="m-1"
                  button-color="black"
                  icon="refresh"
                  :text="trans('servers.update')"
              ></ServerControlButton>

              <ServerControlButton
                  command="reinstall"
                  v-if="serverStore.canUpdate"
                  :server-id="serverId"
                  button="m-1"
                  button-color="black"
                  icon="rcon"
                  :text="trans('servers.reinstall')"
              ></ServerControlButton>
            </div>

            <div class="ml-auto">
              <GStatusBadge
                  :status="serverOnline ? 'success' : 'error'"
                  :text="serverOnline ? trans('servers.active') : trans('servers.inactive')"
              />
            </div>
          </div>
        </n-card>
      </div>

      <div v-if="canShowStats" class="mt-2">
        <ServerStatisticsStrip :server-id="serverId" @open="statsModalShow = true" />
      </div>

      <div class="md:flex md:flex-wrap mt-2" v-show="serverQueryOnline">
        <div class="md:w-full">
          <n-card size="small" class="mb-3">
            <Loading v-if="loading"></Loading>
            <ServerStatus v-if="!loading" ref="serverStatusRef" :server-id="serverId"></ServerStatus>
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
        <GIcon name="rcon" class="mr-1" />
        RCON
      </template>

      <div class="flex flex-wrap mt-2">
        <div class="md:w-full">
          <div :class="'md:grid ' + (showRconConsolePanel && showRconPlayersPanel ? 'md:grid-cols-2' : 'md:grid-cols-1')">
            <div v-if="showRconPlayersPanel" :class="showRconConsolePanel ? 'pr-8' : ''">
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

            <div v-if="showRconConsolePanel">
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
        <GIcon name="folder-solid" class="mr-1" />
        {{ trans('servers.files') }}
      </template>

      <div class="flex flex-wrap mt-2 h-[calc(100vh-14rem)]">
        <div class="md:w-full h-full">
          <div
              class="flex flex-col min-w-0 h-full rounded break-words border bg-surface-raised"
          >
            <FileManager
                :settings="{
                    'lang': pageLanguage(),
                    'baseUrl': '/api/file-manager/'+$route.params.id,
                    'serverName': server?.name || '',
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
        <GIcon name="calendar" class="mr-1" />
        {{ trans('servers.task_scheduler') }}
      </template>

      <ServerTasks
          :server-id="serverId"
          :privileges="privileges"
      ></ServerTasks>
    </n-tab-pane>

    <n-tab-pane
        v-for="tab in pluginTabs"
        :key="'plugin-' + tab.pluginId + '-' + (tab.name || 'tab')"
        :name="'plugin-' + tab.pluginId + '-' + (tab.name || 'tab')"
    >
      <template #tab>
        <GIcon v-if="tab.icon" :name="tab.icon" class="mr-1"></GIcon>
        {{ pluginsStore.resolvePluginText(tab.pluginId, tab.label) }}
      </template>
      <component
          :is="tab.component"
          :server-id="serverId"
          :server="server"
          :plugin-id="tab.pluginId"
      />
    </n-tab-pane>

    <n-tab-pane name="settings" v-if="serverStore.canManageSettings">
      <template #tab>
        <GIcon name="cogs" class="mr-1" />
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
          <GIcon name="hammer" class="mr-1" />
          {{ trans('servers.admin') }}
        </router-link>
      </div>
    </template>

  </n-tabs>

  <ServerStatisticsModal
      v-model:show="statsModalShow"
      :server-id="serverId"
      :server-name="server?.name"
      :online="serverOnline"
  />
</template>

<script setup>
import {computed, h, defineAsyncComponent, onMounted, ref, watch} from "vue";
import {useRoute, useRouter} from "vue-router";
import {storeToRefs} from "pinia";
import ServerControlButton from "./servertabs/ServerControlButton.vue";
import ServerStatisticsStrip from "./servertabs/ServerStatisticsStrip.vue";

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

const ServerStatisticsModal = defineAsyncComponent(() =>
    import('./servertabs/ServerStatisticsModal.vue' /* webpackChunkName: "components/server" */)
)

const RconPlayers = defineAsyncComponent(() =>
    import('../components/rcon/RconPlayers.vue' /* webpackChunkName: "components/rcon" */)
)

const RconConsole = defineAsyncComponent(() =>
    import('../components/rcon/RconConsole.vue' /* webpackChunkName: "components/rcon" */)
)

const FileManager = defineAsyncComponent(() =>
    import('../filemanager/FileManager.vue')
)

import { GBreadcrumbs, Loading, GIcon, GGameIcon } from "@gameap/ui";

import {useServerStore} from "@/store/server"
import {useServerRconStore} from "@/store/serverRcon"
import {useAuthStore} from "@/store/auth"
import {usePluginsStore} from "@/store/plugins"
import {providePluginContext} from "@/plugins/context"
import {trans, pageLanguage} from "@/i18n/i18n";
import InactiveServer from "./InactiveServer.vue";

const route = useRoute()
const router = useRouter()
const serverStore = useServerStore()
const serverRconStore = useServerRconStore()
const authStore = useAuthStore()
const pluginsStore = usePluginsStore()

providePluginContext()

const activeTab = ref('control')
const statsModalShow = ref(false)
const initialHash = route.hash
const initialTabName = (() => {
  if (!initialHash || initialHash === '#' || initialHash === '#control') return 'control'
  return initialHash.startsWith('#') ? initialHash.slice(1) : initialHash
})()
const pendingPluginTab = ref(initialTabName.startsWith('plugin-') ? initialTabName : '')

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
      serverRconStore.fetchRconSupportedFeatures().then(() => {
        setInitialTabFromHash()
      })
    } else {
      setInitialTabFromHash()
    }
  })
  serverStore.fetchAbilities().then(() => {
    setInitialTabFromHash()
  })
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

const canShowStats = computed(() => {
  return serverStore.abilities['game-server-common'] && serverStore.canViewMetrics && serverOnline.value
})

const serverQueryOnline = computed(() => {
  return serverStatusRef.value?.status === 'online'
})

const showRconConsolePanel = computed(() => {
  return serverRconStore.canUseRcon && rconSupportedFeatures.value.rcon
})

const showRconPlayersPanel = computed(() => {
  // playersManage is the pre-split name of playersList; the fallback keeps a cached bundle
  // working against a newer server.
  const canList = rconSupportedFeatures.value.playersList ?? rconSupportedFeatures.value.playersManage

  return serverRconStore.canManageRconPlayers && canList
})

const rconTabPossible = computed(() => {
  return (showRconConsolePanel.value || showRconPlayersPanel.value) && serverOnline.value
})

const breadcrumbs = computed(() => {
  const bc = [
    {'route':'/', 'text':'GameAP', 'icon': 'gameap'},
    {'route':{name: 'servers'}, 'text':trans('servers.game_servers')},
  ]

  if (server.value?.name) {
    bc.push({
      render: () => [
        h(GGameIcon, {game: server.value.game?.code ?? '', class: 'mr-2 align-middle'}),
        h('span', {game: server.value.game?.code ?? '', class: 'align-middle'}, server.value.name),
      ]
    })
  }

  return bc
})

const isAdmin = computed(() => {
  return authStore.isAdmin
})

const pluginTabs = computed(() => {
  const allTabs = pluginsStore.getSlotComponents('server-tabs')
  return allTabs.filter(tab => {
    if (!matchesTabGame(tab.checkGame)) {
      return false
    }
    if (!tab.checkPermission) return true
    if (tab.checkPermission.type === 'hasServerPermissions') {
      return tab.checkPermission.permissions.every(
          perm => serverStore.abilities[perm] === true
      )
    }
    return true
  })
})

function matchesTabGame(checkGame) {
  if (!checkGame) return true
  const game = server.value?.game
  if (!game) return false
  const engines = checkGame.engines ?? []
  const codes = checkGame.codes ?? []
  if (engines.length === 0 && codes.length === 0) return true
  const engineMatches = engines.some(
      engine => String(engine).toLowerCase() === String(game.engine ?? '').toLowerCase()
  )
  const codeMatches = codes.includes(game.code)
  return engineMatches || codeMatches
}

function hashToTabName(hash) {
  if (!hash || hash === '#' || hash === '#control') {
    return 'control'
  }
  return hash.startsWith('#') ? hash.slice(1) : hash
}

function tabNameToHash(tabName) {
  if (!tabName || tabName === 'control') {
    return ''
  }
  return '#' + tabName
}

function getAvailableTabNames() {
  const tabs = ['control']
  if (rconTabPossible.value) tabs.push('rcon')
  if (serverStore.canManageFiles) tabs.push('files')
  if (serverStore.canManageTasks) tabs.push('schedules')
  if (serverStore.canManageSettings) tabs.push('settings')

  pluginTabs.value.forEach(tab => {
    tabs.push('plugin-' + tab.pluginId + '-' + (tab.name || 'tab'))
  })
  return tabs
}

function isValidTabName(tabName) {
  return getAvailableTabNames().includes(tabName)
}

function onTabChange(tabName) {
  activeTab.value = tabName
  const hash = tabNameToHash(tabName)
  if (hash !== (route.hash || '')) {
    router.replace({ ...route, hash })
  }
}

function setInitialTabFromHash() {
  const tabName = hashToTabName(route.hash)
  if (tabName === 'statistics') {
    activeTab.value = 'control'
    if (canShowStats.value) {
      statsModalShow.value = true
      if (route.hash) router.replace({ ...route, hash: '' })
    }
    return
  }
  if (isValidTabName(tabName)) {
    activeTab.value = tabName
    pendingPluginTab.value = ''
  } else if (tabName.startsWith('plugin-')) {
    pendingPluginTab.value = tabName
  } else {
    activeTab.value = 'control'
  }
}

watch(() => route.hash, (newHash) => {
  const tabName = hashToTabName(newHash)
  if (tabName === 'statistics') {
    activeTab.value = 'control'
    if (canShowStats.value) {
      statsModalShow.value = true
      router.replace({ ...route, hash: '' })
    }
    return
  }
  if (isValidTabName(tabName) && activeTab.value !== tabName) {
    activeTab.value = tabName
    pendingPluginTab.value = ''
  } else if (tabName.startsWith('plugin-')) {
    pendingPluginTab.value = tabName
  }
})

watch(
  [
    () => pluginsStore.slots['server-tabs'].length,
    () => Object.keys(serverStore.abilities).length
  ],
  () => {
    if (pendingPluginTab.value && isValidTabName(pendingPluginTab.value)) {
      activeTab.value = pendingPluginTab.value
      pendingPluginTab.value = ''
    }
  },
  { immediate: true }
)

</script>