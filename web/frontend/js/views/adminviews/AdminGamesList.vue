<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <div class="mb-5">
    <GButton class="mr-1" color="green" v-on:click="onClickGameCreate()">
      <i class="fa-solid fa-plus-square"></i>&nbsp;{{ trans('games.add') }}
    </GButton>

    <GButton class="mr-1" color="orange" v-on:click="onClickModCreate()">
      <i class="fa-solid fa-cat"></i>&nbsp;{{ trans('games.add_mod') }}
    </GButton>

    <GButton class="mr-1" color="black" v-on:click="onClickGamesUpgrade()">
      <i class="fa-solid fa-sync"></i>&nbsp{{ trans('games.upgrade')}}
    </GButton>
  </div>

  <div class="w-1/4 mb-1">
    <n-input-group>
      <n-input-group-label>
        <i class="fa-solid fa-magnifying-glass"></i>
      </n-input-group-label>
      <n-input
          v-model:value="searchGames"
          type="text"
          :placeholder="trans('main.search')"
      />
    </n-input-group>
  </div>

  <n-data-table
      ref="tableRef"
      :bordered="false"
      :single-line="true"
      :columns="columns"
      :data="gamesData"
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

  <n-modal
      v-model:show="gameCreateModalEnabled"
      class="custom-card"
      preset="card"
      :title="trans('games.title_add')"
      :bordered="false"
      style="width: 600px"
      :segmented="{content: 'soft', footer: 'soft'}"
  >
    <CreateGameForm
        v-model="gameCreateModel"
        v-on:create="onCreateGame"/>
  </n-modal>

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
import GBreadcrumbs from "../../components/GBreadcrumbs.vue"
import {computed, ref, onMounted, h} from "vue"
import {trans} from "../../i18n/i18n"
import GButton from "../../components/GButton.vue"
import Loading from "../../components/Loading.vue"
import GameIcon from "../../components/GameIcon.vue"
import {useGameListStore} from "../../store/gameList"
import {errorNotification, notification} from "../../parts/dialogs"
import {
  NEmpty,
  NDataTable,
  NModal,
  NInput,
  NInputGroup,
  NInputGroupLabel,
} from "naive-ui"
import {useRouter} from "vue-router"
import {storeToRefs} from "pinia"
import GDeletableList from "../../components/GDeletableList.vue";
import CreateModForm from "./forms/CreateModForm.vue";
import CreateGameForm from "./forms/CreateGameForm.vue";

const router = useRouter()

const gamesStore = useGameListStore()

const breadcrumbs = computed(() => {
  return [
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
    {'route':{name: 'admin.games.index'}, 'text':trans('games.games')},
  ]
})

onMounted(() => {
  fetchGames()
  fetchAllGameMods()
})

const fetchGames = () => {
  gamesStore.fetchGames().catch((error) => {
    errorNotification(error)
  })
}

const fetchAllGameMods = () => {
  gamesStore.fetchAllGameMods().catch((error) => {
    errorNotification(error)
  })
}

const createColumns = () => {
  return [
    {
      title: trans('games.name'),
      key: 'name',
      render(row) {
        return h("div", {class: 'flex items-center'}, [
          h(GameIcon, {game: row.code, class: "mr-2"}),
          h("span", {class: ''}, row.name)
        ])
      },
    },
    {
      title: trans('games.code'),
      key: 'code',
    },
    {
      title: trans('games.mods'),
      key: 'mods',
      render(row) {
        if (row.mods.length === 0) {
          return h(GButton, {
            color: 'orange',
            size: 'small',
            class: 'px-2 py-1',
            onClick: () => {onClickModCreate(row.code)},
          }, [
            h("i", {class: 'fa-solid fa-cat mr-0.5'}),
            h("span", {class: ''}, trans('games.add_first_mod'))
          ])
        }

        return h(
            GDeletableList,
            {
              class: ["md:max-w-48"],
              items: row.mods,
              deleteCallback: onClickModDelete,
              clickCallback: onClickMod,
            }
        )
      },
    },
    {
      title: trans('main.actions'),
      render(row) {
        return [
          h(GButton, {
            color: 'blue',
            size: 'small',
            class: 'mr-0.5',
            route: {name: 'admin.games.edit', params: {code: row.code}},
          }, [
            h("i", {class: 'fa-solid fa-pen-to-square'}),
            h("span", {class: 'hidden lg:inline'}, trans('main.edit')),
          ]),
          h(GButton, {
            color: 'red',
            size: 'small',
            text: trans('main.delete'),
            onClick: () => {onClickGameDelete(row.code)},
          }, [
            h("i", {class: 'fa-solid fa-trash'}),
            h("span", {class: 'hidden lg:inline'}, trans('main.delete')),
          ]),
        ]
      },
    }
  ]
}

const {loading, games, allGameMods} = storeToRefs(gamesStore)

const modCreateModalEnabled = ref(false)
const modCreateModel = ref({
  game: null,
  name: '',
  remoteRepositoryLinux: '',
  remoteRepositoryWindows: '',
})

const gameCreateModalEnabled = ref(false)
const gameCreateModel = ref({
  code: '',
  name: '',
  engine: '',
  engineVersion: '',
  remoteRepositoryLinux: '',
  remoteRepositoryWindows: '',
})


const columns = ref(createColumns())
const pagination = {
  pageSize: 50,
};

const searchGames = ref('')

const gamesData = computed(() => {
  let result = []

  games.value.forEach((game) => {
    if (
        searchGames.value &&
        (
            !game.name.toLowerCase().includes(searchGames.value.toLowerCase()) &&
            !game.code.toLowerCase().includes(searchGames.value.toLowerCase())
        )
    ) {
      return
    }

    result.push({
      name: game.name,
      code: game.code,
      engine: game.engine,
      mods: getGameMods(game.code),
    })
  })

  return result.sort((a, b) => a.name.localeCompare(b.name))
})

const getGameMods = (gameCode) => {
  let mods = []
  allGameMods.value.forEach((gameMod) => {
    if (gameMod.game_code === gameCode) {
      mods.push({
        id: gameMod.id,
        name: gameMod.name,
        gameCode: gameMod.game_code,
      })
    }
  })

  return mods
}

const onClickGameDelete = (code) => {
  window.$dialog.success({
    title: trans('games.delete_game_confirm_msg'),
    positiveText: trans('main.yes'),
    negativeText: trans('main.no' ),
    closable: false,
    onPositiveClick: () => {
      deleteGameByCode(code)
    },
    onNegativeClick: () => {}
  })
}

const deleteGameByCode = (code) => {
  gamesStore.deleteGameByCode(code).then(() => {
    fetchGames()
  }).catch((error) => {
    errorNotification(error)
  })
}

const onClickModCreate = (game) => {
  modCreateModel.value = {
    game: null,
    name: '',
    remoteRepositoryLinux: '',
    remoteRepositoryWindows: '',
  }

  if (game) {
    modCreateModel.value.game = game
  }

  modCreateModalEnabled.value = true
}

const onCreateMod = () => {
  const fields = {
    name: modCreateModel.value.name,
    game_code: modCreateModel.value.game,
    remote_repository_linux: modCreateModel.value.remoteRepositoryLinux,
    remote_repository_windows: modCreateModel.value.remoteRepositoryWindows,
  }

  gamesStore.createGameMod(fields).then(({id}) => {
    notification({
      content: trans('games.mod_create_success_msg'),
      type: "success",
    }, () => {
      fetchAllGameMods()
    })
  }).catch((error) => {
    errorNotification(error)
  }).finally(() => {
    modCreateModalEnabled.value = false
  })
}

const onClickGameCreate = () => {
  gameCreateModel.value = {
    code: '',
    name: '',
    remoteRepositoryLinux: '',
    remoteRepositoryWindows: '',
  }

  gameCreateModalEnabled.value = true
}

const onCreateGame = () => {
  const fields = {
    name: gameCreateModel.value.name,
    code: gameCreateModel.value.code,
    engine: gameCreateModel.value.engine,
    engine_version: gameCreateModel.value.engineVersion,
    remote_repository_linux: gameCreateModel.value.remoteRepositoryLinux,
    remote_repository_windows: gameCreateModel.value.remoteRepositoryWindows,
  }

  if (!fields.engine) {
    fields.engine = "unknown"
  }

  gamesStore.createGame(fields).then(({id}) => {
    notification({
      content: trans('games.create_success_msg'),
      type: "success",
    }, () => {
      gamesStore.fetchGames()
    })
  }).catch((error) => {
    errorNotification(error)
  }).finally(() => {
    gameCreateModalEnabled.value = false
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
  gamesStore.deleteModById(id).then(() => {
    fetchAllGameMods()
  }).catch((error) => {
    errorNotification(error)
  })
}

const onClickGamesUpgrade = () => {
  window.$dialog.success({
    title: trans('games.d_upgrade_confirm'),
    positiveText: trans('main.yes'),
    negativeText: trans('main.no' ),
    closable: false,
    onPositiveClick: () => {
      gamesStore.upgradeGames().then(() => {
        notification({
          content: trans('games.upgrade_success_msg'),
          type: "success",
        })

        fetchGames()
        fetchAllGameMods()

      }).catch((error) => {
        errorNotification(error)
      })
    },
    onNegativeClick: () => {}
  })
}
</script>