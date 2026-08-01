<template>
  <GBreadcrumbs :items="breadcrumbs" />

  <div class="w-full">
    <n-tabs v-model:value="activeTab" type="line" animated>
      <n-tab-pane name="gameap">
        <template #tab>GameAP</template>
        <div class="mt-2">
          <ImportGameAPForm ref="gameapFormRef" @import="onImportGameAP" />
        </div>
      </n-tab-pane>
      <n-tab-pane name="pelican">
        <template #tab>Pelican/Pterodactyl</template>
        <div class="mt-2">
          <ImportPelicanEggForm ref="pelicanFormRef" @import="onImportPelicanEgg" />
        </div>
      </n-tab-pane>
    </n-tabs>
  </div>
</template>

<script setup>
import { GBreadcrumbs } from "@gameap/ui"
import { computed, ref } from "vue"
import { trans } from "@/i18n/i18n"
import { NTabs, NTabPane } from "naive-ui"
import { useRouter } from "vue-router"
import { useGameListStore } from "@/store/gameList"
import { errorNotification, notification } from "@/parts/dialogs"
import ImportPelicanEggForm from "./forms/ImportPelicanEggForm.vue"
import ImportGameAPForm from "./forms/ImportGameAPForm.vue"

const router = useRouter()
const gamesStore = useGameListStore()

const activeTab = ref('gameap')
const pelicanFormRef = ref(null)
const gameapFormRef = ref(null)

const breadcrumbs = computed(() => {
  return [
    { route: '/', text: 'GameAP', icon: 'gameap' },
    { route: { name: 'admin.games.index' }, text: trans('games.games') },
    { route: { name: 'admin.games.import' }, text: trans('games.title_import') },
  ]
})

const onImportGameAP = ({ content, name, code }) => {
  gamesStore.importGameAP(content, { name, code }).then((response) => {
    const msg = trans('games.import_gameap_success_msg')
        .replace(':game_name', response.game_name)
        .replace(':game_code', response.game_code)
        .replace(':mods_imported', response.mods_imported)

    notification({
      content: msg,
      type: "success",
    })

    router.push({ name: 'admin.games.index' })
  }).catch((error) => {
    if (gameapFormRef.value) {
      gameapFormRef.value.setImporting(false)
    }
    errorNotification(error)
  })
}

const onImportPelicanEgg = ({ content, format, name, code }) => {
  gamesStore.importPelicanEgg(content, format, { name, code }).then((response) => {
    const msg = trans('games.import_pelican_egg_success_msg')
        .replace(':game_name', response.game_name)
        .replace(':game_code', response.game_code)

    notification({
      content: msg,
      type: "success",
    })

    router.push({ name: 'admin.games.index' })
  }).catch((error) => {
    if (pelicanFormRef.value) {
      pelicanFormRef.value.setImporting(false)
    }
    errorNotification(error)
  })
}
</script>
