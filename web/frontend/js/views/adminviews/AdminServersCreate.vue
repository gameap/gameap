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
                <GIcon name="dice" />
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

      <div class="md:w-full">
        <n-card
            :title="trans('servers.additional_settings')"
            size="small"
            class="mb-3"
            header-class="g-card-header"
            :segmented="{
                            content: true,
                            footer: 'soft'
                          }"
        >
          <template #header-extra>
            <n-button text @click="showAdditionSettings = !showAdditionSettings">
              {{ showAdditionSettings ? trans('main.hide') : trans('main.show') }}
            </n-button>
          </template>

          <n-collapse-transition :show="showAdditionSettings">
            <n-form-item :label="trans('servers.install')" path="install">
              <GSwitch v-model:value="serverForm.install" />
            </n-form-item>

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

            <div v-if="gameModVars.length > 0">
              <n-divider>{{ trans('games.vars') }}</n-divider>
              <VarFormItem
                  v-for="definition in gameModVars"
                  :key="definition.name"
                  :definition="definition"
                  :path="`settings[${JSON.stringify(definition.name)}]`"
                  :value="serverForm.settings[definition.name] ?? null"
                  @update:value="serverForm.settings[definition.name] = $event"
              />
            </div>
          </n-collapse-transition>
        </n-card>
      </div>

      <GFixedBottomBar>
        <GButton color="green" v-on:click="onClickCreate">
          <GIcon name="add-square" class="mr-0.5" />
          <span class="inline">{{ trans('main.create') }}</span>
        </GButton>
      </GFixedBottomBar>
    </div>
  </n-form>
</template>

<script setup>
import { GBreadcrumbs, GIcon, GSwitch } from "@gameap/ui"
import {computed, onMounted, ref, watch} from "vue"
import {trans} from "@/i18n/i18n"
import {useGameListStore} from "@/store/gameList"
import {useNodeListStore} from "@/store/nodeList"
import {useServerListStore} from "@/store/serverList"
import {storeToRefs} from "pinia"
import {errorNotification, notification} from "@/parts/dialogs"
import {NForm, NFormItem, NInputGroup, NDivider} from "naive-ui"
import {generateServerName} from "@/parts/nameGenerator"
import GButton from "@/components/GButton.vue"
import {useRouter} from "vue-router";
import {requiredValidator} from "@/parts/validators";
import {useGameModStore} from "@/store/gameMod"
import DsIpSelector from "@/components/servers/DsIpSelector.vue";
import SmartPortSelector from "@/components/servers/SmartPortSelector.vue";
import GameModSelector from "@/components/servers/GameModSelector.vue";
import GFixedBottomBar from "@/components/GFixedBottomBar.vue";
import VarFormItem from "@/components/input/VarFormItem.vue";
import {coerceValue, isBlankValue, normalizeVarDefinition, serializeValue} from "@/parts/gameModVars";

const router = useRouter()

const gamesStore = useGameListStore()
const nodeListStore = useNodeListStore()
const serverListStore = useServerListStore()
const gameModStore = useGameModStore()

const {games} = storeToRefs(gamesStore)
const {nodes} = storeToRefs(nodeListStore)
const {mod: gameMod} = storeToRefs(gameModStore)

const formRef = ref({})
const serverForm = ref({
  serverPort: 27015,
  queryPort: 27015,
  rconPort: 27015,
  install: true,
  user: 'gameap',
  settings: {},
})
const showAdditionSettings = ref(false)

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gameap'},
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

watch(() => serverForm.value.gameMod, async (newModId) => {
  serverForm.value.settings = {}
  if (!newModId) {
    return
  }

  gameModStore.setModId(newModId)

  try {
    await gameModStore.fetchMod()
  } catch (error) {
    // Without the definitions the form has no widgets and no defaults, so the
    // failure is reported instead of leaving an empty settings section behind.
    errorNotification(error)

    return
  }

  // The selection may have changed while the mod was loading; seeding then
  // would fill the form with another mod's switches.
  if (serverForm.value.gameMod !== newModId) {
    return
  }

  // Text and number fields stay blank so the placeholder shows the mod default
  // and the backend applies it; a switch has no blank state, so it is seeded.
  const seeded = {}
  for (const definition of gameModVars.value) {
    if (definition.type === 'bool') {
      seeded[definition.name] = coerceValue(definition, definition.default)
    }
  }
  serverForm.value.settings = seeded
})

const gameModVars = computed(() => (gameMod.value?.vars || []).map(normalizeVarDefinition))

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
      // The variables live inside a collapsed block that stays mounted, so an
      // invalid one would otherwise block the submit with nothing to look at.
      showAdditionSettings.value = true

      notification({
        content: trans('servers.settings_check_form'),
        type: "error",
      })
    } else {
      createServer()
    }
  });
}

const createServer = () => {
  // A switch always carries a value; everything else is only sent when filled in,
  // so an untouched field falls back to the mod default.
  const settings = gameModVars.value
    .map((definition) => ({definition, value: serverForm.value.settings?.[definition.name]}))
    .filter(({definition, value}) => definition.type === 'bool' || !isBlankValue(value))
    .map(({definition, value}) => ({
      name: definition.name,
      value: serializeValue(definition, value),
    }))

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
    settings: settings.length > 0 ? settings : undefined,
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