<template>
  <nav class="fixed z-50 top-0 w-full bg-chrome">
    <div class="w-full px-2 sm:px-6 lg:px-8">
      <div class="relative flex h-16 items-center justify-between">

        <div class="absolute inset-y-0 left-0 flex items-center sm:hidden">
          <!-- Mobile menu button-->
          <button
              type="button"
              @click="showMobileMenu = !showMobileMenu"
              class="relative inline-flex items-center justify-center rounded-md p-2 text-stone-400 hover:bg-chrome-hover hover:text-white focus:outline-none focus:ring-2 focus:ring-inset focus:ring-white"
              aria-controls="mobile-menu"
              aria-expanded="false"
          >
            <span class="absolute -inset-0.5"></span>
            <span class="sr-only">Open main menu</span>
            <!--
              Icon when menu is closed.

              Menu open: "hidden", Menu closed: "block"
            -->
            <svg class="block h-6 w-6" fill="none" viewBox="0 0  24 24" stroke-width="1.5" stroke="currentColor" aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5" />
            </svg>
            <!--
              Icon when menu is open.

              Menu open: "block", Menu closed: "hidden"
            -->
            <svg class="hidden h-6 w-6" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor" aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <div class="flex flex-1 items-center sm:items-stretch sm:justify-start ml-12 sm:ml-0">
          <div class="flex flex-shrink-0 items-center">
            <a id="brand-link" class="navbar-brand" href="/">
              <img src="/images/gap_logo_white_mini.png" class="logo-mini sm:hidden" alt="GameAP">
              <img src="/images/gap_logo_white.png" class="logo hidden sm:block" alt="GameAP">
            </a>
          </div>
        </div>

        <div class="flex items-center">
          <div class="flex items-center md:mr-4 gap-x-1.5 text-white hover:bg-chrome-item px-5 py-2 rounded cursor-pointer" v-on:click="switchTheme()">
            <GIcon v-if="currentTheme === 'dark'" name="sun" />
            <GIcon v-if="currentTheme === 'light'" name="moon" />
          </div>

          <MainNavbarDropdown
              class="md:mr-4"
              :button-text="trans('navbar.help')"
              button-icon="help"
              :items="[
                  [
                      {
                        icon: 'book',
                        label: trans('navbar.documentation'),
                        link: pageLanguage === 'ru' ? 'https://docs.gameap.com/ru/' : 'https://docs.gameap.com/en/',
                      },
                      {
                        icon: 'admin-panel',
                        label: trans('navbar.api_documentation'),
                        link: 'https://openapi.gameap.io/',
                      }
                  ],
                ]"
          ></MainNavbarDropdown>

          <MainNavbarDropdown
              class="md:mr-4"
              :button-text="user.name"
              button-icon="user"
              :items="[
                  [
                      {
                        icon: 'profile',
                        label: trans('navbar.profile'),
                        route: {name: 'profile'},
                      },
                      {
                        icon: 'key',
                        label: trans('tokens.tokens'),
                        route: {name: 'tokens'},
                      }
                  ],
                  [
                      {
                        icon: 'logout',
                        label: trans('navbar.sign_out'),
                        onClick: logout,
                      }
                  ]
                ]"
          ></MainNavbarDropdown>
        </div>
      </div>
    </div>

    <!-- Mobile menu, show/hide based on menu state. -->
    <div class="sm:hidden" v-if="showMobileMenu">
      <div class="space-y-1 px-2 pb-3 pt-2">
        <router-link
            v-for="link in serversLinks"
            :key="'server-' + link.route.name"
            @click="showMobileMenu = !showMobileMenu"
            :to="link.route"
            class="bg-chrome-item text-white flex items-center rounded px-3 py-2 font-medium"
            aria-current="page"
        >
          <GIcon :name="link.icon" class="ml-1 shrink-0" />
          <span class="ml-2">{{ link.text }}</span>
        </router-link>
        <router-link
            v-for="item in pluginServersMenuItems"
            :key="'plugin-server-' + item.pluginId + '-' + item.text"
            @click="showMobileMenu = !showMobileMenu"
            :to="item.route"
            class="bg-chrome-item text-white flex items-center rounded px-3 py-2 font-medium"
            aria-current="page"
        >
          <GIcon :name="item.icon" class="ml-1 shrink-0" />
          <span class="ml-2">{{ pluginsStore.resolvePluginText(item.pluginId, item.text) }}</span>
        </router-link>

        <template v-if="isAdmin">
          <router-link
              v-for="link in adminLinks"
              :key="'admin-' + link.route.name"
              @click="showMobileMenu = !showMobileMenu"
              :to="link.route"
              class="bg-chrome-item text-white flex items-center rounded px-3 py-2 font-medium"
              aria-current="page"
          >
            <GIcon :name="link.icon" class="ml-1 shrink-0" />
            <span class="ml-2">{{ link.text }}</span>
          </router-link>
          <router-link
              v-for="item in pluginAdminMenuItems"
              :key="'plugin-admin-' + item.pluginId + '-' + item.text"
              @click="showMobileMenu = !showMobileMenu"
              :to="item.route"
              class="bg-chrome-item text-white flex items-center rounded px-3 py-2 font-medium"
              aria-current="page"
          >
            <GIcon :name="item.icon" class="ml-1 shrink-0" />
            <span class="ml-2">{{ pluginsStore.resolvePluginText(item.pluginId, item.text) }}</span>
          </router-link>
        </template>

        <template v-for="(items, section) in customPluginSections" :key="'section-' + section">
          <div class="text-stone-400 px-3 py-1 text-sm">{{ section }}</div>
          <router-link
              v-for="item in items"
              :key="'plugin-custom-' + item.pluginId + '-' + item.text"
              @click="showMobileMenu = !showMobileMenu"
              :to="item.route"
              class="bg-chrome-item text-white flex items-center rounded px-3 py-2 font-medium"
              aria-current="page"
          >
            <GIcon :name="item.icon" class="ml-1 shrink-0" />
            <span class="ml-2">{{ pluginsStore.resolvePluginText(item.pluginId, item.text) }}</span>
          </router-link>
        </template>
      </div>
    </div>
  </nav>
</template>

<script setup>
import {trans, pageLanguage} from "@/i18n/i18n"
import {computed, ref} from 'vue'
import MainNavbarDropdown from "./MainNavbarDropdown.vue";
import {adminLinks, serversLinks} from "./bars";
import {useAuthStore} from "@/store/auth";
import {useUISettingsStore} from "@/store/uiSettings";
import {usePluginsStore} from "@/store/plugins";
import {errorNotification} from "@/parts/dialogs";

const authStore = useAuthStore()
const uiSettingsStore = useUISettingsStore()
const pluginsStore = usePluginsStore()

const user = computed(() => {
    return authStore.user
})

const currentTheme = computed(() => {
    return uiSettingsStore.currentTheme
})

const showMobileMenu = ref(false)

const isAdmin = computed(() => authStore.isAdmin)

const pluginServersMenuItems = computed(() => pluginsStore.getMenuItems('servers'))

const pluginAdminMenuItems = computed(() => pluginsStore.getMenuItems('admin'))

const customPluginSections = computed(() => {
    const items = pluginsStore.getMenuItems('custom')
    return items.reduce((acc, item) => {
        const section = item.section || 'Plugins'
        if (!acc[section]) acc[section] = []
        acc[section].push(item)
        return acc
    }, {})
})

const switchTheme = () => {
    uiSettingsStore.toggleTheme()
}

const logout = () => {
    authStore.logout().then(() => {
        window.location.href = '/'
    }).catch((error) => {
        errorNotification(error)
    })
}
</script>