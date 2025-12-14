<template>
    <div class="mt-4">
      <GButton class="px-8 py-8 bg-stone-50 hover:bg-stone-100
        border dark:border-stone-700 dark:bg-stone-800
        dark:hover:bg-stone-900 m-2 h-28 min-w-48" :route="{name: 'servers'}">
        <div class="text-lg">
          <i class="fa-solid fa-server"></i>
          {{ trans('servers.servers') }}
        </div>
        <div class="mt-3 grid grid-cols-2 content-center">
          <div class="text-xs text-lime-600">
            <div class="inline" v-if="!loading">
              <i class="fa-solid fa-heart-pulse"></i>:
            </div>
            <i v-if="loading" class="fa-solid fa-gear fa-spin"></i>
            <span v-else>{{ serverListStore.summary.online }}</span>
          </div>
          <div class="text-xs text-red-600">
            <div class="inline" v-if="!loading">
              <i class="fa-solid fa-skull"></i>:
            </div>
            <i v-if="loading" class="fa-solid fa-gear fa-spin"></i>
            <span v-else>{{ serverListStore.summary.offline }}</span>
          </div>
        </div>
      </GButton>

      <GButton v-if="isAdmin" class="px-8 py-8 bg-stone-50 hover:bg-stone-100
        px-8 py-8 bg-stone-50 hover:bg-stone-100 border dark:border-stone-700
        dark:bg-stone-800 dark:hover:bg-stone-900 m-2 h-28 min-w-48" :route="{name: 'admin.nodes.index'}">
        <div class="text-lg">
          <i class="fa-solid fa-hard-drive"></i>
          {{ trans('dedicated_servers.nodes' )}}
        </div>
        <div class="mt-3 grid grid-cols-2 content-center">
          <div class="text-xs text-lime-600">
            <div class="inline" v-if="!loading">
              <i class="fa-solid fa-heart-pulse"></i>:
            </div>
            <i v-if="loading" class="fa-solid fa-gear fa-spin"></i>
            <span v-else>{{ nodeListStore.summary.online }}</span>
          </div>
          <div class="text-xs text-red-600">
            <div class="inline" v-if="!loading">
              <i class="fa-solid fa-power-off"></i>:
            </div>
            <i v-if="loading" class="fa-solid fa-gear fa-spin"></i>
            <span v-else>{{ nodeListStore.summary.offline }}</span>
          </div>
        </div>
      </GButton>

      <template v-for="btn in homeButtons" :key="btn.pluginId + '-' + btn.name">
        <component
            v-if="btn.component"
            :is="btn.component"
            v-bind="btn.props"
            :plugin-id="btn.pluginId"
        />
        <GButton
            v-else
            class="px-8 py-8 bg-stone-50 hover:bg-stone-100
              border dark:border-stone-700 dark:bg-stone-800
              dark:hover:bg-stone-900 m-2 h-28 min-w-48"
            :route="btn.props.route"
        >
          <div class="text-lg">
            <i :class="btn.icon"></i>
            {{ pluginsStore.resolvePluginText(btn.pluginId, btn.label) }}
          </div>
        </GButton>
      </template>
    </div>


  <div class="w-full mt-10 p-3 border border-stone-200 bg-stone-50 rounded-lg sm:p-4 dark:bg-stone-800 dark:border-stone-700">
    <div class="grid grid-cols-5 gap-4">
      <h5 class="col-span-3 text-base inline-block align-middle font-semibold text-stone-900 dark:text-white">
        <i class="fa-regular fa-comments mr-1"></i>
        {{ trans('home.channels') }}
      </h5>
      <div class="col-span-2">
        <a
            type="button"
            class="text-white bg-[#24A1DE] hover:bg-[#24A1DE]/90 focus:outline-none rounded text-sm px-2 py-1 text-center inline-flex items-center me-1 mb-1 mr-2"
            href="https://t.me/gameap"
            target="_blank"
        >
          <i class="fa-brands fa-telegram mr-1"></i>
          Telegram
        </a>

        <a
            type="button"
            class="text-white bg-[#7289da] hover:bg-[#7289da]/90 focus:outline-none rounded text-sm px-2 py-1 text-center inline-flex items-center me-1 mb-1 mr-2"
            href="https://discord.gg/SqtHpZc"
            target="_blank"
        >
          <i class="fa-brands fa-discord mr-1"></i>
          Discord
        </a>

        <a
            v-if="pageLanguage() === 'ru'"
            type="button"
            class="text-white bg-[#0077FF] hover:bg-[#0077FF]/90 focus:outline-none rounded text-sm px-2 py-1 text-center inline-flex items-center me-1 mb-1 mr-2"
            href="https://vk.com/gameap"
            target="_blank"
        >
          <i class="fa-brands fa-vk mr-1"></i>
          VKontakte
        </a>

        <a
            type="button"
            class="text-white bg-[#FF5700] hover:bg-[#FF5700]/90 focus:outline-none rounded text-sm px-2 py-1 text-center inline-flex items-center me-1 mb-1 mr-2"
            href="https://www.reddit.com/r/gameap/"
            target="_blank"
        >
          <i class="fa-brands fa-reddit mr-1"></i>
          Reddit
        </a>

      </div>
    </div>
  </div>

  <div class="w-full mt-5 p-3 border border-stone-200 bg-stone-50 rounded-lg sm:p-4 dark:bg-stone-800 dark:border-stone-700">
    <div class="grid grid-cols-5 gap-4">
      <h5 class="col-span-3 text-base inline-block align-middle font-semibold text-stone-900 dark:text-white">
        <i class="fa-regular fa-heart mr-1"></i>
        {{ trans('home.support_gameap') }}
      </h5>
      <div class="col-span-2">
        <div class="flex">
          <div>
            <a
                type="button"
                class="text-white bg-[#F96854] hover:bg-[#F96854]/90 focus:outline-none rounded text-sm px-2 py-1 text-center inline-flex items-center me-1 mb-1 mr-2"
                href="https://www.patreon.com/gameap"
                target="_blank"
            >
              <i class="fa-brands fa-patreon mr-1"></i>
              Patreon
            </a>
          </div>

          <div>
            <a
                type="button"
                class="text-white bg-[#F15F2C] hover:bg-[#F15F2C]/90 focus:outline-none rounded text-sm px-2 py-1 text-center inline-flex items-center me-1 mb-1 mr-2"
                href="https://boosty.to/gameap"
                target="_blank"
            >
              <svg class="w-5 h-5" viewBox="0 0 235.6 292.2">
                <path class="st0" fill="#FFFFFF" d="M44.3,164.5L76.9,51.6H127l-10.1,35c-0.1,0.2-0.2,0.4-0.3,0.6L90,179.6h24.8c-10.4,25.9-18.5,46.2-24.3,60.9
		c-45.8-0.5-58.6-33.3-47.4-72.1 M90.7,240.6l60.4-86.9h-25.6l22.3-55.7c38.2,4,56.2,34.1,45.6,70.5
		c-11.3,39.1-57.1,72.1-101.7,72.1C91.3,240.6,91,240.6,90.7,240.6z"/>
              </svg>

              Boosty
            </a>
          </div>
        </div>
      </div>
    </div>
  </div>

  <div class="w-full mt-5 p-3 border border-stone-200 bg-stone-50 rounded-lg sm:p-4 dark:bg-stone-800 dark:border-stone-700">
    <div class="grid grid-cols-5 gap-4">
      <h5 class="col-span-3 text-base inline-block align-middle font-semibold text-stone-900 dark:text-white">
        <i class="fa-regular fa-circle-question mr-1"></i>
        {{ trans('home.need_help') }}
      </h5>
      <div class="col-span-2">
        <a
            type="button"
            class="text-white bg-sky-500 hover:bg-sky-400 focus:outline-none rounded text-sm px-2 py-1 text-center inline-flex items-center me-1 mb-1 mr-2"
            :href=" (pageLanguage() === 'ru') ? 'https://docs.gameap.com/ru/' : 'https://docs.gameap.com/en/'"
            target="_blank"
        >
          <i class="fa-solid fa-book mr-1"></i>
          {{ trans('home.documentation') }}
        </a>

        <a
            type="button"
            class="text-white bg-sky-500 hover:bg-sky-400 focus:outline-none rounded text-sm px-2 py-1 text-center inline-flex items-center me-1 mb-1 mr-2"
            href="https://openapi.gameap.io/"
            target="_blank"
        >
          <i class="fa-solid fa-book mr-1"></i>
          {{ trans('home.api_documentation') }}
        </a>

        <a
            type="button"
            class="text-white bg-[#7289da] hover:bg-[#7289da]/90 focus:outline-none rounded text-sm px-2 py-1 text-center inline-flex items-center me-1 mb-1 mr-2"
            href="https://discord.gg/SqtHpZc"
            target="_blank"
        >
          <i class="fa-brands fa-discord mr-1"></i>
          Discord
        </a>
      </div>
    </div>
  </div>

  <PluginSlot v-if="pluginsStore.isInitialized" name="dashboard-widgets" :context="{ isAdmin }" />
</template>

<script setup>
import {computed, onMounted} from "vue"
import GButton from "../components/GButton.vue"
import PluginSlot from "../plugins/components/PluginSlot.vue"
import {useAuthStore} from "../store/auth"
import {usePluginsStore} from "../store/plugins"
import {useNodeListStore} from "../store/nodeList"
import {useServerListStore} from "../store/serverList"
import {trans, pageLanguage} from "../i18n/i18n"
import {errorNotification} from "../parts/dialogs";

const authStore = useAuthStore()
const nodeListStore = useNodeListStore()
const serverListStore = useServerListStore()
const pluginsStore = usePluginsStore()

const isAdmin = computed(() => {
  return authStore.isAdmin
})

const loading = computed(() => {
  return serverListStore.loading || nodeListStore.loading
})

const homeButtons = computed(() => {
  return pluginsStore.getSlotComponents('home-buttons')
})

onMounted(() => {
  fetchServersSummary()

  if (isAdmin.value) {
    fetchNodesSummary()
  }
})

const fetchServersSummary = () => {
  serverListStore.fetchServersSummary().catch((error) => {
    errorNotification(error)
  })
}

const fetchNodesSummary = () => {
  nodeListStore.fetchNodesSummary().catch((error) => {
    errorNotification(error)
  })
}

</script>