<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <GButton class="mr-1" color="orange" v-on:click="onClickModCreate()">
    <GIcon name="cat" />&nbsp;{{ trans('games.add_mod') }}
  </GButton>

  <GButton class="mr-1" color="green" v-on:click="onClickExport()">
    <GIcon name="export" />&nbsp;{{ trans('games.export') }}
  </GButton>

  <UpdateGameForm :loading="loading" v-model="gameUpdateModel" v-on:update="onClickUpdate">
    <template #mods>
      <n-card
          :title="trans('games.mods')"
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
          <GDeletableList
              :items="modItems"
              :deleteCallback="onClickModDelete"
              :clickCallback="onClickMod"
          />
        </div>
      </n-card>
    </template>
  </UpdateGameForm>

  <GModal
      v-model:show="modCreateModalEnabled"
      :title="trans('games.title_add_mod')"
      style="width: 600px"
  >
    <CreateModForm
        v-model="modCreateModel"
        v-on:create="onCreateMod"
    />
  </GModal>
</template>

<script setup>
import { GBreadcrumbs, GDeletableList, GIcon, Loading, GModal } from "@gameap/ui"
import UpdateGameForm from "./forms/UpdateGameForm.vue"
import {computed, ref} from "vue"
import {trans} from "../../i18n/i18n";
import GButton from "../../components/GButton.vue";
import {useGameStore} from "../../store/game"
import {useGameListStore} from "../../store/gameList";
import {useInitialLoad} from "../../composables/useInitialLoad";
import CreateModForm from "./forms/CreateModForm.vue";
import {errorNotification, notification} from "../../parts/dialogs";
import { NCard } from "naive-ui"
import {useRoute, useRouter} from "vue-router"
import {storeToRefs} from "pinia"

const route = useRoute()
const router = useRouter()
const gameStore = useGameStore()
const gameListStore = useGameListStore()

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gameap'},
    {'route':{name: 'admin.games.index'}, 'text':trans('games.games')},
    {'text': trans('games.title_edit')}
  ]
})

const {gameCode, game, mods} = storeToRefs(gameStore)

const loading = useInitialLoad(async () => {
  gameStore.setGameCode(route.params.code)
  gameUpdateModel.value.code = gameCode.value

  const [gameResult, modsResult] = await Promise.allSettled([
    gameStore.fetchGame(),
    gameStore.fetchMods(),
  ])

  if (modsResult.status === 'rejected') {
    errorNotification(modsResult.reason)
  }

  if (gameResult.status === 'rejected') {
    errorNotification(gameResult.reason)

    return
  }

  gameUpdateModel.value.name = game.value.name
  gameUpdateModel.value.engine = game.value.engine
  gameUpdateModel.value.engineVersion = game.value.engine_version

  gameUpdateModel.value.steamAppIdLinux = game.value.steam_app_id_linux
  gameUpdateModel.value.steamAppIdWindows = game.value.steam_app_id_windows
  gameUpdateModel.value.steamAppSetConfig = game.value.steam_app_set_config

  gameUpdateModel.value.localRepositoryLinux = game.value.local_repository_linux
  gameUpdateModel.value.localRepositoryWindows = game.value.local_repository_windows

  gameUpdateModel.value.remoteRepositoryLinux = game.value.remote_repository_linux
  gameUpdateModel.value.remoteRepositoryWindows = game.value.remote_repository_windows

  gameUpdateModel.value.metadata = Object.entries(game.value.metadata || {})
      .map(([key, value]) => ({
        key,
        value: typeof value === 'object' ? JSON.stringify(value, null, 2) : String(value)
      }))
})

const modItems = computed(() => {
  let items = []
  mods.value.forEach((gameMod) => {
    items.push({
      id: gameMod.id,
      name: gameMod.name,
      gameCode: gameCode.value,
    })
  })

  return items
})

const onClickModCreate = (game) => {
  gameListStore.fetchGames().then(() => {
    modCreateModel.value = {
      game: gameCode.value,
      name: '',
      remoteRepositoryLinux: '',
      remoteRepositoryWindows: '',
    }

    if (game) {
      modCreateModel.value.game = game
    }

    modCreateModalEnabled.value = true
  }).catch((error) => {
    errorNotification(error)
  })
}

const gameUpdateModel = ref({})

const modCreateModalEnabled = ref(false)
const modCreateModel = ref({
  game: null,
  name: '',
  remoteRepositoryLinux: '',
  remoteRepositoryWindows: '',
})

const onCreateMod = () => {
  const fields = {
    name: modCreateModel.value.name,
    game_code: modCreateModel.value.game,
    remote_repository_linux: modCreateModel.value.remoteRepositoryLinux,
    remote_repository_windows: modCreateModel.value.remoteRepositoryWindows,
  }

  gameListStore.createGameMod(fields).then(({id}) => {
    notification({
      content: trans('games.mod_create_success_msg'),
      type: "success",
    }, () => {
      gameListStore.fetchAllGameMods()
    })
  }).catch((error) => {
    errorNotification(error)
  }).finally(() => {
    modCreateModalEnabled.value = false
  })
}

const onClickMod = (code, id) => {
  router.push({name: 'admin.games.mods.edit', params: {code: code, id: id}})
}

const onClickModDelete = (id) => {
  window.$dialog.success({
    title: trans('games.delete_mod_confirm_msg'),
    positiveText: trans('main.yes'),
    negativeText: trans('main.no' ),
    closable: false,
    onPositiveClick: () => {
      deleteModById(id)
    },
    onNegativeClick: () => {}
  })
}

const deleteModById = (id) => {
  gameListStore.deleteModById(id).then(() => {
    gameStore.fetchMods()
  }).catch((error) => {
    errorNotification(error)
  })
}

const onClickUpdate = () => {
  const metadataObj = {}
  for (const {key, value} of gameUpdateModel.value.metadata || []) {
    if (key) {
      try {
        metadataObj[key] = JSON.parse(value)
      } catch {
        metadataObj[key] = value
      }
    }
  }

  const fields = {
    name: gameUpdateModel.value.name,
    engine: gameUpdateModel.value.engine,
    engine_version: gameUpdateModel.value.engineVersion,
    steam_app_id_linux: gameUpdateModel.value.steamAppIdLinux,
    steam_app_id_windows: gameUpdateModel.value.steamAppIdWindows,
    steam_app_set_config: gameUpdateModel.value.steamAppSetConfig,
    local_repository_linux: gameUpdateModel.value.localRepositoryLinux,
    local_repository_windows: gameUpdateModel.value.localRepositoryWindows,
    remote_repository_linux: gameUpdateModel.value.remoteRepositoryLinux,
    remote_repository_windows: gameUpdateModel.value.remoteRepositoryWindows,
    metadata: metadataObj,
  }

  gameStore.saveGame(fields).then(() => {
    notification({
      content: trans('games.update_success_msg'),
      type: "success",
    }, () => {
      router.push({name: 'admin.games.index'})
    })
  }).catch((error) => {
    errorNotification(error)
  })
}

const onClickExport = () => {
  gameListStore.exportGame(gameCode.value).then(() => {
    notification({
      content: trans('games.export_success_msg'),
      type: "success",
    })
  }).catch((error) => {
    errorNotification(error)
  })
}
</script>