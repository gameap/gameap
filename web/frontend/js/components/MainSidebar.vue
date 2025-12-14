<template>
    <!-- Component Start -->
    <div v-if="minimized === true" class="items-center w-16 mr-5"></div>
    <div v-if="minimized === true" class="sidebar-menu fixed items-center w-16 h-full overflow-y-scroll no-scrollbar text-stone-400 bg-stone-900">
        <a class="flex items-center w-full px-3 mt-3" href="#">
          <span class="ml-2 w-full text-center text-sm font-bold">—</span>
        </a>

        <div class="w-full px-2">
          <div class="flex flex-col items-center w-full mb-3 border-stone-700">
            <router-link v-for="link in serversLinks" :to="link.route" class="flex items-center transition transform w-full h-10 px-3 mt-2 bg-stone-800 hover:translate-x-2">
              <i :class="link.icon" class="ml-1"></i>
            </router-link>
            <router-link
                v-for="item in pluginServersMenuItems"
                :key="item.pluginId + '-' + item.text"
                :to="item.route"
                class="flex items-center transition transform w-full h-10 px-3 mt-2 bg-stone-800 hover:translate-x-2"
            >
              <i :class="item.icon" class="ml-1"></i>
            </router-link>
          </div>
        </div>

        <a v-if="isAdmin" class="flex items-center w-full px-3 mt-3" href="#">
          <span class="ml-2 w-full text-center text-sm font-bold">—</span>
        </a>

        <div v-if="isAdmin" class="w-full px-2">
          <div class="flex flex-col items-center w-full mb-3 border-stone-700">
            <router-link v-for="link in adminLinks" :to="link.route" class="flex items-center transition transform w-full h-10 px-3 mt-2 bg-stone-800 hover:translate-x-2">
              <i :class="link.icon" class="ml-1"></i>
            </router-link>
            <router-link
                v-for="item in pluginAdminMenuItems"
                :key="item.pluginId + '-' + item.text"
                :to="item.route"
                class="flex items-center transition transform w-full h-10 px-3 mt-2 bg-stone-800 hover:translate-x-2"
            >
              <i :class="item.icon" class="ml-1"></i>
            </router-link>
          </div>
        </div>

        <template v-for="(items, section) in customPluginSections" :key="section">
          <a class="flex items-center w-full px-3 mt-3" href="#">
            <span class="ml-2 w-full text-center text-sm font-bold">—</span>
          </a>
          <div class="w-full px-2">
            <div class="flex flex-col items-center w-full mb-3 border-stone-700">
              <router-link
                  v-for="item in items"
                  :key="item.pluginId + '-' + item.text"
                  :to="item.route"
                  class="flex items-center transition transform w-full h-10 px-3 mt-2 bg-stone-800 hover:translate-x-2"
              >
                <i :class="item.icon" class="ml-1"></i>
              </router-link>
            </div>
          </div>
        </template>

        <div class="w-full px-2 mt-3">
          <div class="flex flex-col items-center w-full mb-3 border-stone-700">
            <a v-on:click="toggleMinimized" class="flex items-center transition transform w-full h-10 px-3 mt-2 bg-stone-800 hover:translate-x-2">
              <i class="fas fa-chevron-right ml-1"></i>
            </a>
          </div>
        </div>

        <div class="mb-20"></div>

      </div>
    <!-- Component End  -->

    <!-- Component Start -->
    <div v-if="minimized === false" class="items-center w-56 mr-5"></div>
    <div v-if="minimized === false" class="sidebar-menu fixed items-center w-56 h-full overflow-y-scroll no-scrollbar text-stone-400 bg-stone-900">
      <a class="flex items-center w-full px-3 mt-3" href="#">
        <span class="ml-2 w-full text-center text-sm font-bold">{{ trans('sidebar.control') }}</span>
      </a>

      <div class="w-full px-2">
        <div class="flex flex-col items-center w-full mb-3 border-stone-700">
          <template v-for="link in serversLinks">
            <router-link :to="link.route" class="flex items-center transition transform w-full h-10 px-3 mt-2 bg-stone-800 hover:translate-x-2">
              <i :class="link.icon" class="ml-1"></i>
              <span class="ml-2 text-sm font-medium">{{ link.text }}</span>
            </router-link>
          </template>
          <router-link
              v-for="item in pluginServersMenuItems"
              :key="item.pluginId + '-' + item.text"
              :to="item.route"
              class="flex items-center transition transform w-full h-10 px-3 mt-2 bg-stone-800 hover:translate-x-2"
          >
            <i :class="item.icon" class="ml-1"></i>
            <span class="ml-2 text-sm font-medium">{{ pluginsStore.resolvePluginText(item.pluginId, item.text) }}</span>
          </router-link>
        </div>
      </div>

      <a v-if="isAdmin" class="flex items-center w-full px-3 mt-3" href="#">
        <span class="ml-2 w-full text-center text-sm font-bold">{{ trans('sidebar.admin') }}</span>
      </a>

      <div v-if="isAdmin" class="w-full px-2">
        <div class="flex flex-col items-center w-full mb-3 border-stone-700">
          <router-link v-for="link in adminLinks" :to="link.route" class="flex items-center transition transform w-full h-10 px-3 mt-2 bg-stone-800 hover:translate-x-2">
            <i :class="link.icon" class="ml-1"></i>
            <span class="ml-2 text-sm font-medium">{{ link.text }}</span>
          </router-link>
          <router-link
              v-for="item in pluginAdminMenuItems"
              :key="item.pluginId + '-' + item.text"
              :to="item.route"
              class="flex items-center transition transform w-full h-10 px-3 mt-2 bg-stone-800 hover:translate-x-2"
          >
            <i :class="item.icon" class="ml-1"></i>
            <span class="ml-2 text-sm font-medium">{{ pluginsStore.resolvePluginText(item.pluginId, item.text) }}</span>
          </router-link>
        </div>
      </div>

      <template v-for="(items, section) in customPluginSections" :key="section">
        <a class="flex items-center w-full px-3 mt-3" href="#">
          <span class="ml-2 w-full text-center text-sm font-bold">{{ section }}</span>
        </a>
        <div class="w-full px-2">
          <div class="flex flex-col items-center w-full mb-3 border-stone-700">
            <router-link
                v-for="item in items"
                :key="item.pluginId + '-' + item.text"
                :to="item.route"
                class="flex items-center transition transform w-full h-10 px-3 mt-2 bg-stone-800 hover:translate-x-2"
            >
              <i :class="item.icon" class="ml-1"></i>
              <span class="ml-2 text-sm font-medium">{{ pluginsStore.resolvePluginText(item.pluginId, item.text) }}</span>
            </router-link>
          </div>
        </div>
      </template>

      <div class="w-full px-2 mt-3">
        <div class="flex flex-col items-center w-full mb-3 border-stone-700">
          <a v-on:click="toggleMinimized" class="flex items-center transition transform w-full h-10 px-3 mt-2 bg-stone-800 hover:translate-x-2" href="#">
            <i class="fas fa-chevron-left ml-1"></i>
            <span class="ml-2 text-sm font-medium">{{ trans('sidebar.minimize') }}</span>
          </a>
        </div>
      </div>

      <div class="mb-20"></div>

    </div>
    <!-- Component End  -->

</template>

<script setup>

import {trans} from "@/i18n/i18n";
import {ref, computed} from "vue";
import {adminLinks, serversLinks} from "./bars";
import {useAuthStore} from "@/store/auth";
import {useUISettingsStore} from "@/store/uiSettings";
import {usePluginsStore} from "@/store/plugins";

const authStore = useAuthStore()
const uiSettingsStore = useUISettingsStore()
const pluginsStore = usePluginsStore()

const minimized = computed({
  get: () => uiSettingsStore.isMenuMinimized,
  set: (value) => uiSettingsStore.setMenuMinimized(value)
})

function toggleMinimized() {
  uiSettingsStore.toggleMenuMinimized();

  document.querySelectorAll('.sidebar-menu').forEach((el) => {
    el.scrollTop = 0;
  })
}

const isAdmin = computed(() => {
  return authStore.isAdmin
})

const pluginServersMenuItems = computed(() => {
  return pluginsStore.getMenuItems('servers')
})

const pluginAdminMenuItems = computed(() => {
  return pluginsStore.getMenuItems('admin')
})

const customPluginSections = computed(() => {
  const items = pluginsStore.getMenuItems('custom')
  return items.reduce((acc, item) => {
    const section = item.section || 'Plugins'
    if (!acc[section]) acc[section] = []
    acc[section].push(item)
    return acc
  }, {})
})
</script>

<style scoped>
/* Hide scrollbar for Chrome, Safari and Opera */
.no-scrollbar::-webkit-scrollbar {
  display: none;
}
/* Hide scrollbar for IE, Edge and Firefox */
.no-scrollbar {
  -ms-overflow-style: none;  /* IE and Edge */
  scrollbar-width: none;  /* Firefox */
}
</style>