<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <GButton class="mr-1" color="orange" v-on:click="onClickModCreate()">
    <i class="fa-solid fa-cat"></i>&nbsp;{{ trans('games.add_mod') }}
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

  <n-modal
      v-model:show="modCreateModalEnabled"
      class="custom-card"
      preset="card"
      :title="trans('games.title_add_mod')"
      :bordered="false"
      style="width: 600px"
      :segmented="{content: 'soft', footer: 'soft'}"
  >
    <CreateModForm
        v-model="modCreateModel"
        v-on:create="onCreateMod"
    />
  </n-modal>
</template>

<script setup>
import UpdateGameForm from "./forms/UpdateGameForm.vue"
import GBreadcrumbs from "../../components/GBreadcrumbs.vue"
import {computed, ref, onMounted} from "vue"
import {trans} from "../../i18n/i18n";
import GButton from "../../components/GButton.vue";
import {useGameStore} from "../../store/game"
import {useGameListStore} from "../../store/gameList";
import CreateModForm from "./forms/CreateModForm.vue";
import {errorNotification, notification} from "../../parts/dialogs";
import {
  NModal,
  NCard,
} from "naive-ui"
import {useRoute, useRouter} from "vue-router"
import {storeToRefs} from "pinia"
import GDeletableList from "../../components/GDeletableList.vue";
import Loading from "../../components/Loading.vue";

const route = useRoute()
const router = useRouter()
const gameStore = useGameStore()
const gameListStore = useGameListStore()

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
    {'route':{name: 'admin.games.index'}, 'text':trans('games.games')},
    {'text': trans('games.title_edit')}
  ]
})

const {gameCode, game, mods} = storeToRefs(gameStore)

onMounted(() => {
  gameStore.setGameCode(route.params.code)
  gameUpdateModel.value.code = gameCode.value

  gameStore.fetchGame().then(() => {
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
  })

  gameStore.fetchMods()
})

const loading = computed(() => {
  return gameStore.loading || gameListStore.loading
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
</script>