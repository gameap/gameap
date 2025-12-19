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
          <n-form-item :label="trans('labels.name')" path="name">
            <n-input-group>
              <n-input
                  v-model:value="serverForm.name"
                  type="text"
              />
              <n-button @click="generateRandomName">
                <i class="fa-solid fa-dice"></i>
              </n-button>
            </n-input-group>
          </n-form-item>

          <GameModSelector
              :games="gamesCodeName"
              game-path="game"
              game-mod-path="gameMod"
              v-model:game="serverForm.game"
              v-model:game-mod="serverForm.gameMod"
          ></GameModSelector>

          <n-form-item :label="trans('servers.install')" path="install">
            <n-switch
                v-model:value="serverForm.install"
            >
            </n-switch>
          </n-form-item>

          <n-collapse>
            <n-collapse-item :title="trans('main.more')">

              <n-form-item :label="trans('labels.rcon')" path="rcon">
                <n-input
                    v-model:value="serverForm.rcon"
                    type="password"
                    show-password-on="click"
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

            </n-collapse-item>
          </n-collapse>
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
          <DsIpSelector
              :ds-list="nodesIdName"
              v-model:node-id="serverForm.nodeId"
              v-model:ip="serverForm.ip"
              node-id-path="nodeId"
              ip-path="ip"
          >
          </DsIpSelector>
          <SmartPortSelector
              v-model:server-port="serverForm.serverPort"
              v-model:rcon-port="serverForm.rconPort"
              v-model:query-port="serverForm.queryPort"
              server-port-path="serverPort"
              rcon-port-path="rconPort"
              query-port-path="queryPort"
          ></SmartPortSelector>
        </n-card>
      </div>

      <GButton color="green" v-on:click="onClickCreate">
        <i class="fa-regular fa-square-plus"></i>
        <span class="hidden lg:inline">&nbsp;{{ trans('main.create') }}</span>
      </GButton>
    </div>
  </n-form>
</template>

<script setup>
import { GBreadcrumbs } from "@gameap/ui"
import {computed, onMounted, ref, watch} from "vue"
import {trans} from "@/i18n/i18n"
import {useGameListStore} from "@/store/gameList"
import {useNodeListStore} from "@/store/nodeList"
import {useServerListStore} from "@/store/serverList"
import {storeToRefs} from "pinia"
import {errorNotification, notification} from "@/parts/dialogs"
import {NForm, NFormItem, NSwitch, NInputGroup} from "naive-ui"
import {generateServerName} from "@/parts/nameGenerator"
import GButton from "@/components/GButton.vue"
import {useRouter} from "vue-router";
import {requiredValidator} from "@/parts/validators";

const router = useRouter()

const gamesStore = useGameListStore()
const nodeListStore = useNodeListStore()
const serverListStore = useServerListStore()

const {games} = storeToRefs(gamesStore)
const {nodes} = storeToRefs(nodeListStore)

const formRef = ref({})
const serverForm = ref({
  serverPort: 27015,
  queryPort: 27015,
  rconPort: 27015,
  install: true,
  user: 'gameap',
})

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
    {'route':{name: 'admin.servers.index'}, 'text':trans('servers.game_servers')},
    {'route':{name: 'admin.servers.create'}, 'text':trans('servers.create')},
  ]
})

onMounted(() => {
  fetchGames()
  fetchNodes()
})

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

const fetchGames = async () => {
  try {
    await gamesStore.fetchGames()
  } catch (e) {
    errorNotification(error)
  }
}

const fetchNodes = () => {
  nodeListStore.fetchNodesByFilter([]).
  catch((error) => {
    errorNotification(error)
  })
}

watch(nodesIdName, (newNodes) => {
  const nodeIds = Object.keys(newNodes)
  if (nodeIds.length === 1 && !serverForm.value.nodeId) {
    serverForm.value.nodeId = Number(nodeIds[0])
  }
}, { immediate: true })

watch(() => serverForm.value.name, (newName) => {
  if (!newName || serverForm.value.game) {
    return
  }

  const words = newName.toLowerCase().split(/\s+/).filter(w => w.length >= 3)
  if (words.length === 0) {
    return
  }

  const matchedGames = new Set()

  for (const [code, name] of Object.entries(gamesCodeName.value)) {
    const codeLower = code.toLowerCase()
    const nameLower = name.toLowerCase()

    for (const word of words) {
      if (codeLower.includes(word) || word.includes(codeLower) ||
          nameLower.includes(word) || word.includes(nameLower)) {
        matchedGames.add(code)
        break
      }
    }
  }

  if (matchedGames.size === 1) {
    serverForm.value.game = [...matchedGames][0]
  }
})

const generateRandomName = () => {
  const gameName = gamesCodeName.value[serverForm.value.game] || 'Server'
  serverForm.value.name = generateServerName(gameName)
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
  installed: {
    required: true,
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

const onClickCreate = () => {
  formRef.value?.validate((errors, { warnings }) => {
    if (errors) {
      console.log(errors)
      notification({
        content: "Please check the form.",
        type: "error",
      })
    } else {
      createServer()
    }
  });
}

const createServer = () => {
  serverListStore.create({
    name: serverForm.value.name,
    game_id: serverForm.value.game,
    game_mod_id: serverForm.value.gameMod,
    install: serverForm.value.install,
    rcon: serverForm.value.rcon,
    su_user: serverForm.value.user,
    ds_id: serverForm.value.nodeId,
    server_ip: serverForm.value.ip,
    server_port: serverForm.value.serverPort,
    query_port: serverForm.value.queryPort,
    rcon_port: serverForm.value.rconPort,
    dir: serverForm.value.dir,
  }).
  then(() => {
    notification({
      content: trans('servers.create_success_msg'),
      type: "success",
    }, () => {
      router.push({name: 'admin.servers.index'})
    })
  }).catch((error) => {
    errorNotification(error)
  })
}
</script>