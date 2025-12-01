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
            <div class="grid grid-cols-3">
              <div class="pr-8">
                <n-form-item :label="trans('servers.status')">
                  <n-select
                      v-model:value="serverForm.status"
                      :options="statusOptions"
                  />
                </n-form-item>
              </div>


              <n-form-item :label="trans('labels.enabled')" path="enabled">
                <n-switch
                    v-model:value="serverForm.enabled"
                >
                </n-switch>
              </n-form-item>

              <n-form-item :label="trans('labels.blocked')" path="blocked">
                <n-switch
                    v-model:value="serverForm.blocked"
                    :rail-style="({checked}) => { return checked ? {background: '#b91c1c'} : {}}"
                >
                </n-switch>
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
            ></GameModSelector>

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
                :initial-server-ip="server.server_ip"
                :initial-server-port="server.server_port"
                :initial-query-port="server.query_port"
                :initial-rcon-port="server.rcon_port"
                server-port-path="serverPort"
                rcon-port-path="rconPort"
                query-port-path="queryPort"
            ></SmartPortSelector>
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

      <GButton color="green" v-on:click="onClickSave">
        <i class="fa-solid fa-floppy-disk"></i>
        <span class="hidden lg:inline">&nbsp;{{ trans('main.save') }}</span>
      </GButton>
    </div>
  </n-form>
</template>

<script setup>
import GBreadcrumbs from "@/components/GBreadcrumbs.vue"
import {computed, ref, onMounted} from "vue"
import {trans} from "@/i18n/i18n"
import {NForm, NFormItem, NSwitch} from "naive-ui"
import GButton from "@/components/GButton.vue"
import {useRoute, useRouter} from "vue-router"
import {storeToRefs} from "pinia"
import { capitalize } from "lodash-es"
import {errorNotification, notification} from "@/parts/dialogs";
import {useServerStore} from "@/store/server"
import {useGameListStore} from "@/store/gameList"
import {useNodeListStore} from "@/store/nodeList"
import {requiredValidator} from "@/parts/validators";
import Loading from "@/components/Loading.vue";

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
})

const {games} = storeToRefs(gamesStore)
const {nodes} = storeToRefs(nodeListStore)
const {server} = storeToRefs(serverStore)

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
    {'route':{name: 'admin.servers.index'}, 'text':trans('servers.game_servers')},
    {'route':{name: 'admin.servers.edit', params: {id: route.params.id}}, 'text':trans('servers.edit')},
  ]
})

onMounted(() => {
  serverStore.setServerId(Number(route.params.id))

  serverStore.fetchServer().then(() => {
    serverForm.value.name = server.value.name
    serverForm.value.ip = server.value.server_ip
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

    fetchGames().then(() => {
      serverForm.value.game = server.value.game_id
      serverForm.value.gameMod = server.value.game_mod_id
    })

    fetchNodes().then(() => {
      serverForm.value.nodeId = server.value.ds_id
    })

  }).catch((error) => {
    errorNotification(error)
  })
})

const loading = computed(() => {
  return serverStore.loading || gamesStore.loading || nodeListStore.loading
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
  } catch (e) {
    errorNotification(error)
  }
}

const fetchNodes = async () => {
  nodeListStore.fetchNodesByFilter([]).
  catch((error) => {
    errorNotification(error)
  })
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
  serverStore.save({
    name: serverForm.value.name,
    game_id: server.value.game_id,
    game_mod_id: server.value.game_mod_id,
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