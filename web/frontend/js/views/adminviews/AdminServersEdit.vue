<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <n-form
      label-placement="top"
      label-width="auto"
      ref="formRef"
      :model="serverForm"
      :rules="rules"
  >
    <div class="flex flex-wrap mt-2">
      <div class="md:w-1/2 pr-8">
        <n-card
            :title="trans('servers.basic_info')"
            size="small"
            class="mb-3"
            header-class="g-card-header"
            :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
        >
          <Loading v-if="loading"></Loading>
          <div :class="loading ? 'hidden' : ''">
            <div class="grid grid-cols-1 md:grid-cols-3">
              <div class="pr-8">
                <n-form-item :label="trans('servers.status')">
                  <n-select
                      v-model:value="serverForm.status"
                      :options="statusOptions"
                  />
                </n-form-item>
              </div>


              <n-form-item :label="trans('labels.enabled')" path="enabled">
                <GSwitch v-model:value="serverForm.enabled" />
              </n-form-item>

              <n-form-item :label="trans('labels.blocked')" path="blocked">
                <GSwitch
                    v-model:value="serverForm.blocked"
                    :rail-style="({checked}) => { return checked ? {background: readThemeVar('--gameap-red-700', '#b91c1c')} : {}}"
                />
              </n-form-item>
            </div>

            <n-form-item :label="trans('labels.name')" path="name">
              <n-input
                  v-model:value="serverForm.name"
                  type="text"
              />
            </n-form-item>

            <GameModSelector
                :games="gamesCodeName"
                game-path="game"
                game-mod-path="gameMod"
                :game-select-disabled="true"
                v-model:game="serverForm.game"
                v-model:game-mod="serverForm.gameMod"
            >
              <template #game-actions>
                <GButton
                    v-if="serverForm.game"
                    color="blue"
                    size="small"
                    :route="{name: 'admin.games.edit', params: {code: serverForm.game}}"
                    :title="trans('main.edit')"
                >
                  <GIcon name="edit" />
                </GButton>
              </template>
              <template #mod-actions>
                <GButton
                    v-if="serverForm.game && serverForm.gameMod"
                    color="blue"
                    size="small"
                    :route="{name: 'admin.games.mods.edit', params: {code: serverForm.game, id: serverForm.gameMod}}"
                    :title="trans('main.edit')"
                >
                  <GIcon name="edit" />
                </GButton>
              </template>
            </GameModSelector>

            <n-form-item :label="trans('labels.rcon')" path="rcon">
              <n-input
                  v-model:value="serverForm.rcon"
                  type="password"
                  show-password-on="click"
                  :input-props="{ autocomplete: 'one-time-code' }"
              />
            </n-form-item>

            <n-form-item :label="trans('labels.dir')" class="mb-4" path="dir">
              <n-input
                  v-model:value="serverForm.dir"
                  type="text"
              >
              </n-input>
              <template #feedback>
                <small v-html="trans('servers.d_dir')"></small>
              </template>
            </n-form-item>

            <n-form-item :label="trans('labels.su_user')" path="user">
              <n-input
                  v-model:value="serverForm.user"
                  type="text"
              />
            </n-form-item>
          </div>

        </n-card>
      </div>

      <div class="md:w-1/2">
        <n-card
            :title="trans('servers.ds_ip_ports')"
            size="small"
            class="mb-3"
            header-class="g-card-header"
            :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
        >
          <Loading v-if="loading"></Loading>
          <div :class="loading ? 'hidden' : ''">
            <DsIpSelector
                :ds-list="nodesIdName"
                v-model:node-id="serverForm.nodeId"
                v-model:ip="serverForm.ip"
                :node-select-disabled="true"
                node-id-path="nodeId"
                ip-path="ip"
            >
            </DsIpSelector>
            <SmartPortSelector
                v-model:server-port="serverForm.serverPort"
                v-model:rcon-port="serverForm.rconPort"
                v-model:query-port="serverForm.queryPort"
                :initial-server-ip="server.internal_server_ip ?? server.server_ip"
                :initial-server-port="server.server_port"
                :initial-query-port="server.query_port"
                :initial-rcon-port="server.rcon_port"
                server-port-path="serverPort"
                rcon-port-path="rconPort"
                query-port-path="queryPort"
            ></SmartPortSelector>
          </div>

        </n-card>

        <n-card
            :title="trans('servers.resource_limits')"
            size="small"
            class="mb-3"
            header-class="g-card-header"
            :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
        >
          <Loading v-if="loading"></Loading>
          <div :class="loading ? 'hidden' : ''">
            <div class="grid">
              <n-form-item :label="trans('labels.cpu_limit')">
                <CpuInput v-model="cpuLimit" />
              </n-form-item>
              <n-form-item :label="trans('labels.ram_limit')">
                <MemoryInput v-model="ramLimit" />
              </n-form-item>
            </div>
          </div>
        </n-card>
      </div>

      <div class="md:w-full">
        <n-card
            :title="trans('servers.start_command')"
            size="small"
            class="mb-3"
            header-class="g-card-header"
            :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
        >
          <Loading v-if="loading"></Loading>
          <div :class="loading ? 'hidden' : ''">
            <n-form-item :label="trans('labels.start_command')" path="startCommand">
              <n-input
                  v-model:value="serverForm.startCommand"
                  type="textarea"
                  :autosize="{ minRows: 4}"
              />
            </n-form-item>

            <div class="md:w-full">
              <table class="stone-table">
                <thead class="stone-table-header">
                <tr>
                  <th class="px-2 py-2 w-1/4">{{ trans('labels.name') }}</th>
                  <th class="px-2 py-2">{{ trans('labels.the_value') }}</th>
                </tr>
                </thead>

                <tbody>
                <tr v-for="item in startCommandVars" class="stone-table-row">
                  <td class="px-2 py-2 w-1/4">
                    <span class="bg-stone-100 dark:bg-stone-600 highlighter-rouge p-1 rounded">
                      <span>{</span>{{ item.name }}<span>}</span>
                    </span>
                  </td>
                  <td class="px-2 py-2">{{ item.value }}</td>
                </tr>
                </tbody>
              </table>
            </div>
          </div>

        </n-card>
      </div>

      <div class="md:w-full">
        <n-card
            :title="trans('servers.server_vars')"
            size="small"
            class="mb-3"
            header-class="g-card-header"
            :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
        >
          <Loading v-if="loading"></Loading>
          <div :class="loading ? 'hidden' : ''">
            <InputManyList
                v-model="vars"
                class="mb-4"
                :labels="[trans('labels.key'), trans('labels.the_value')]"
                :keys="['key', 'value']"
                :input-types="['text', 'text']"
            />
          </div>
        </n-card>
      </div>

      <div class="md:w-full">
        <n-card
            :title="trans('servers.metadata')"
            size="small"
            class="mb-3"
            header-class="g-card-header"
            :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
        >
          <Loading v-if="loading"></Loading>
          <div :class="loading ? 'hidden' : ''">
            <InputManyList
                v-model="serverForm.metadata"
                class="mb-4"
                :labels="[trans('labels.key'), trans('labels.the_value')]"
                :keys="['key', 'value']"
                :input-types="['text', 'text']"
                :reference="metadataKeyGroups"
            />
          </div>
        </n-card>
      </div>

      <GFixedBottomBar>
        <GButton color="green" v-on:click="onClickSave">
          <GIcon name="save" />
          <span class="inline">{{ trans('main.save') }}</span>
        </GButton>
      </GFixedBottomBar>
    </div>
  </n-form>
</template>

<script setup>
import { GBreadcrumbs, GIcon, Loading, GSwitch } from "@gameap/ui"
import {computed, ref} from "vue"
import {trans} from "@/i18n/i18n"
import {NForm, NFormItem, NCard} from "naive-ui"
import GButton from "@/components/GButton.vue"
import GFixedBottomBar from "@/components/GFixedBottomBar.vue"
import InputManyList from "@/components/input/InputManyList.vue"
import MemoryInput from "@/components/input/MemoryInput.vue"
import CpuInput from "@/components/input/CpuInput.vue"
import {useRoute, useRouter} from "vue-router"
import {storeToRefs} from "pinia"
import { capitalize } from "lodash-es"
import {errorNotification, notification} from "@/parts/dialogs";
import {useServerStore} from "@/store/server"
import {useGameListStore} from "@/store/gameList"
import {useNodeListStore} from "@/store/nodeList"
import {useInitialLoad} from "@/composables/useInitialLoad"
import {requiredValidator} from "@/parts/validators";
import {readThemeVar} from "@/utils/theme";
import {metadataKeyGroups} from "@/parts/metadataKeys";
import SmartPortSelector from "@/components/servers/SmartPortSelector.vue";
import DsIpSelector from "@/components/servers/DsIpSelector.vue";
import GameModSelector from "@/components/servers/GameModSelector.vue";

const route = useRoute()
const router = useRouter()

const gamesStore = useGameListStore()
const nodeListStore = useNodeListStore()
const serverStore = useServerStore()

const formRef = ref({})
const serverForm = ref({
  serverPort: 27015,
  queryPort: 27015,
  rconPort: 27015,
  install: true,
  user: 'gameap',
  metadata: [],
})

const cpuLimit = ref(null)
const ramLimit = ref(null)
const vars = ref([])

const {games} = storeToRefs(gamesStore)
const {nodes} = storeToRefs(nodeListStore)
const {server} = storeToRefs(serverStore)

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gameap'},
    {'route':{name: 'admin.servers.index'}, 'text':trans('servers.game_servers')},
    {'route':{name: 'admin.servers.edit', params: {id: route.params.id}}, 'text':trans('servers.edit')},
  ]
})

// The games and nodes lists are fetched after the server itself, and the form
// fields that depend on them are filled once both have arrived — the selectors
// need their options before a value is assigned to them.
const loading = useInitialLoad(async () => {
  serverStore.setServerId(Number(route.params.id))

  try {
    await serverStore.fetchServer()
  } catch (error) {
    errorNotification(error)

    return
  }

  serverForm.value.name = server.value.name
  // server_ip carries the published address (public_ip metadata when set), which
  // is what every viewer sees. The form edits the address the daemon connects to.
  serverForm.value.ip = server.value.internal_server_ip ?? server.value.server_ip
  serverForm.value.serverPort = server.value.server_port
  serverForm.value.queryPort = server.value.query_port
  serverForm.value.rconPort = server.value.rcon_port
  serverForm.value.status = server.value.installed
  serverForm.value.enabled = server.value.enabled
  serverForm.value.blocked = server.value.blocked

  serverForm.value.rcon = server.value.rcon
  serverForm.value.dir = server.value.dir
  serverForm.value.user = server.value.su_user
  serverForm.value.startCommand = server.value.start_command

  serverForm.value.metadata = Object.entries(server.value.metadata || {})
      .map(([key, value]) => ({key, value: String(value)}))

  cpuLimit.value = server.value.cpu_limit
  ramLimit.value = server.value.ram_limit
  vars.value = Object.entries(server.value.vars || {})
      .map(([key, value]) => ({key, value: String(value)}))

  await Promise.all([fetchGames(), fetchNodes()])

  serverForm.value.nodeId = server.value.ds_id
  serverForm.value.game = server.value.game_id
  serverForm.value.gameMod = server.value.game_mod_id
})

const statusOptions = [
  {value: 0, label: capitalize(trans('servers.not_installed')) },
  {value: 1, label: capitalize(trans('servers.installed')) },
  {value: 2, label: capitalize(trans('servers.installation')) },
]

const gamesCodeName = computed(() => {
  let result = {}
  for (const [_, value] of Object.entries(games.value)) {
    result[value.code] = value.name
  }
  return result
})

const nodesIdName = computed(() => {
  let result = {}
  for (const [_, value] of Object.entries(nodes.value)) {
    result[value.id] = value.name
  }
  return result
})

const startCommandVars = computed(
    () => {
      let aliases = server.value.aliases

      if (!aliases) {
        aliases = {}
      }

      aliases.ip = serverForm.value.ip
      aliases.port = serverForm.value.serverPort
      aliases.query_port = serverForm.value.queryPort
      aliases.rcon_port = serverForm.value.rconPort
      aliases.rcon_password = serverForm.value.rcon
      aliases.uuid = server.value.uuid
      aliases.uuid_short = server.value.uuid_short

      return Object.entries(aliases).map(([k,v]) => {
        return {name: k, value: v}
      })
    }
)

const fetchGames = async () => {
  try {
    await gamesStore.fetchGames()
  } catch (error) {
    errorNotification(error)
  }
}

const fetchNodes = async () => {
  try {
    await nodeListStore.fetchNodesByFilter([])
  } catch (error) {
    errorNotification(error)
  }
}

const rules = {
  name: {
    required: true,
    validator: requiredValidator(trans('labels.name')),
  },
  game: {
    required: true,
    validator: requiredValidator(trans('labels.game_id'))
  },
  gameMod: {
    required: true,
    validator: requiredValidator(trans('labels.game_mod_id'))
  },
  nodeId: {
    required: true,
    validator: requiredValidator(trans('labels.ds_id'))
  },
  ip: {
    required: true,
    validator: requiredValidator(trans('labels.ip'))
  },
  serverPort: {
    required: true,
    validator: requiredValidator(trans('labels.server_port'))
  },
  queryPort: {
    required: true,
    validator: requiredValidator(trans('labels.query_port'))
  },
  rconPort: {
    required: true,
    validator: requiredValidator(trans('labels.rcon_port'))
  },
}

const onClickSave = () => {
  formRef.value?.validate((errors, { warnings }) => {
    if (errors) {
      console.log(errors)
      notification({
        content: "Please check the form.",
        type: "error",
      })
    } else {
      saveServer()
    }
  });
}

const saveServer = () => {
  const metadataObj = {}
  for (const {key, value} of serverForm.value.metadata || []) {
    if (key) {
      metadataObj[key] = value
    }
  }

  const varsObj = {}
  for (const {key, value} of vars.value || []) {
    if (key) {
      varsObj[key] = value
    }
  }

  serverStore.save({
    name: serverForm.value.name,
    game_id: serverForm.value.game,
    game_mod_id: serverForm.value.gameMod,
    enabled: serverForm.value.enabled,
    blocked: serverForm.value.blocked,
    installed: serverForm.value.status,
    rcon: serverForm.value.rcon,
    ds_id: server.value.ds_id,
    server_ip: serverForm.value.ip,
    server_port: serverForm.value.serverPort,
    query_port: serverForm.value.queryPort,
    rcon_port: serverForm.value.rconPort,
    dir: serverForm.value.dir,
    su_user: serverForm.value.user,
    start_command: serverForm.value.startCommand,
    cpu_limit: cpuLimit.value,
    ram_limit: ramLimit.value,
    vars: varsObj,
    metadata: metadataObj,
  }).
  then(() => {
    notification({
      content: trans('servers.update_success_msg'),
      type: "success",
    }, () => {
      router.push({name: 'admin.servers.index'})
    })
  }).catch((error) => {
    errorNotification(error)
  })
}
</script>