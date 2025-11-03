<template>
  <GBreadcrumbs :items="breadcrumbs"></GBreadcrumbs>

  <Loading v-if="loading"></Loading>
  <div :class="loading ? 'hidden' : ''">
    <UpdateModForm
        v-model="modUpdateModel"
        v-on:update="onUpdateMod"
        :loading="loading"
    />
  </div>
</template>

<script setup>
import GBreadcrumbs from "@/components/GBreadcrumbs.vue"
import Loading from "@/components/Loading.vue"
import {computed, ref, onMounted} from "vue"
import _ from "lodash"
import {trans} from "@/i18n/i18n"
import UpdateModForm from "./forms/UpdateModForm.vue"
import {useGameStore} from "@/store/game"
import {useGameModStore} from "@/store/gameMod"
import {errorNotification, notification} from "@/parts/dialogs"
import {storeToRefs} from "pinia"
import {useRoute, useRouter} from "vue-router"

const route = useRoute()
const router = useRouter()

const gameStore = useGameStore()
const gameModStore = useGameModStore()

const breadcrumbs = computed(() => {
  let result = [
    {'route':'/', 'text':'GameAP', 'icon': 'gicon gicon-gameap'},
    {'route':{name: 'admin.games.index'}, 'text':trans('games.games')},
  ]

  if (game.value.name) {
    result.push(
        {
          route: {name: 'admin.games.edit', params: {code: game.value.code}},
          text: game.value.name,
        }
    )
  }

  if (mod.value.name) {
    result.push(
        {
          route: {name: 'admin.games.mods.edit', params: {code: game.value.code, id: mod.value.id}},
          text: mod.value.name,
        }
    )
  }

  return result
})

const {game} = storeToRefs(gameStore)
const {mod} = storeToRefs(gameModStore)

const loading = computed(() => {
  return gameStore.loading || gameModStore.loading
})

onMounted(() => {
  gameStore.setGameCode(route.params.code)
  gameStore.fetchGame().then(() => {
    gameModStore.setModId(route.params.id)
    gameModStore.fetchMod().then(() => {

      modUpdateModel.value = Object.fromEntries(
          Object.entries(mod.value).map(([k, v]) => [_.camelCase(k), v])
      );
    }).catch((error) => {
      errorNotification(error)
    })

  }).catch((error) => {
    errorNotification(error)
  })
})

const modUpdateModel = ref({})

const onUpdateMod = () => {
  const fields = Object.fromEntries(
      Object.entries(modUpdateModel.value).map(([k, v]) => [_.snakeCase(k), v])
  );
  gameModStore.saveMod(fields).then(() => {
    notification({
      content: trans('games.mod_update_success_msg'),
      type: "success",
    }, () => {
      router.push({name: 'admin.games.index'})
    })
  }).catch((error) => {
    errorNotification(error)
  })
}
</script>