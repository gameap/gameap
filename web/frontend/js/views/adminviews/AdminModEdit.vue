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
import { GBreadcrumbs, Loading } from "@gameap/ui"
import {computed, ref} from "vue"
import { camelCase, snakeCase } from "lodash-es"
import {trans} from "@/i18n/i18n"
import UpdateModForm from "./forms/UpdateModForm.vue"
import {useGameStore} from "@/store/game"
import {useGameModStore} from "@/store/gameMod"
import {useInitialLoad} from "@/composables/useInitialLoad"
import {errorNotification, notification} from "@/parts/dialogs"
import {storeToRefs} from "pinia"
import {useRoute, useRouter} from "vue-router"
import {denormalizeFastRcon, denormalizeVar} from "@/parts/gameModVars"

const route = useRoute()
const router = useRouter()

const gameStore = useGameStore()
const gameModStore = useGameModStore()

const breadcrumbs = computed(() => {
  let result = [
    {'route':'/', 'text':'GameAP', 'icon': 'gameap'},
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

const loading = useInitialLoad(async () => {
  gameStore.setGameCode(route.params.code)
  gameModStore.setModId(route.params.id)

  try {
    await gameStore.fetchGame()
    await gameModStore.fetchMod()
  } catch (error) {
    errorNotification(error)

    return
  }

  modUpdateModel.value = Object.fromEntries(
      Object.entries(mod.value).map(([k, v]) => [camelCase(k), v])
  );

  modUpdateModel.value.metadata = Object.entries(mod.value.metadata || {})
      .map(([key, value]) => ({
        key,
        value: typeof value === 'object' ? JSON.stringify(value, null, 2) : String(value)
      }))
})

const modUpdateModel = ref({})

const onUpdateMod = () => {
  const metadataObj = {}
  for (const {key, value} of modUpdateModel.value.metadata || []) {
    if (key) {
      try {
        metadataObj[key] = JSON.parse(value)
      } catch {
        metadataObj[key] = value
      }
    }
  }

  const fields = Object.fromEntries(
      Object.entries(modUpdateModel.value).map(([k, v]) => [snakeCase(k), v])
  );
  fields.metadata = metadataObj

  // The snake-case mapping above only touches top-level keys, so the array items
  // are normalized here: empty rules dropped, plain-string options restored,
  // fields that do not apply to the type removed.
  fields.vars = (modUpdateModel.value.vars || []).map(denormalizeVar)
  fields.fast_rcon = (modUpdateModel.value.fastRcon || []).map(denormalizeFastRcon)

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