<template>
  <nav class="fixed z-50 top-0 w-full bg-stone-900">
    <div class="w-full px-2 sm:px-6 lg:px-8">
      <div class="relative flex h-16 items-center justify-between">

        <div class="absolute inset-y-0 left-0 flex items-center sm:hidden">
          <!-- Mobile menu button-->
          <button
              type="button"
              @click="showMobileMenu = !showMobileMenu"
              class="relative inline-flex items-center justify-center rounded-md p-2 text-stone-400 hover:bg-stone-700 hover:text-white focus:outline-none focus:ring-2 focus:ring-inset focus:ring-white"
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

        <div class="flex flex-1 items-center justify-center sm:items-stretch sm:justify-start">
          <div class="flex flex-shrink-0 items-center">
            <a id="brand-link" class="navbar-brand" href="/">
              <img id="brand-logo" src="/images/gap_logo_white.png" class="logo" alt="GameAP">
            </a>
          </div>
        </div>

        <div class="md:mr-4 gap-x-1.5 text-white hover:bg-stone-800 md:px-5 md:py-2 rounded" v-on:click="switchTheme()">
          <i v-if="currentTheme === 'dark'" class="fa-solid fa-sun px-2 w-full"></i>
          <i v-if="currentTheme === 'light'" class="fa-solid fa-moon px-2 w-full"></i>
        </div>

        <MainNavbarDropdown
            class="md:mr-4"
            :button-text="trans('navbar.help')"
            button-icon="fa-solid fa-circle-question"
            :items="[
                [
                    {
                      icon: 'fa-solid fa-book',
                      label: trans('navbar.documentation'),
                      link: pageLanguage === 'ru' ? 'https://docs.gameap.com/ru/' : 'https://docs.gameap.com/en/',
                    },
                    {
                      icon: 'fa-solid fa-laptop-code',
                      label: trans('navbar.api_documentation'),
                      link: 'https://openapi.gameap.io/',
                    }
                ],
              ]"
        ></MainNavbarDropdown>

        <MainNavbarDropdown
            class="md:mr-4"
            :button-text="user.name"
            button-icon="fa-solid fa-user"
            :items="[
                [
                    {
                      icon: 'fa-regular fa-address-card',
                      label: trans('navbar.profile'),
                      route: {name: 'profile'},
                    },
                    {
                      icon: 'fa-solid fa-key',
                      label: trans('tokens.tokens'),
                      route: {name: 'tokens'},
                    }
                ],
                [
                    {
                      icon: 'fa-solid fa-sign-out-alt',
                      label: trans('navbar.sign_out'),
                      onClick: logout,
                    }
                ]
              ]"
        ></MainNavbarDropdown>
      </div>
    </div>

    <!-- Mobile menu, show/hide based on menu state. -->
    <div class="sm:hidden" v-if="showMobileMenu">
      <div class="space-y-1 px-2 pb-3 pt-2">
        <router-link
            v-for="link in serversLinks"
            @click="showMobileMenu = !showMobileMenu"
            :to="link.route"
            class="bg-stone-800 text-white block rounded px-3 py-2 font-medium"
            aria-current="page"
        >
          <i :class="link.icon" class="ml-1"></i>
          {{ link.text }}
        </router-link>
        <router-link
            v-for="link in adminLinks"
            @click="showMobileMenu = !showMobileMenu"
            :to="link.route"
            class="bg-stone-800 text-white block rounded px-3 py-2 font-medium"
            aria-current="page"
        >
          <i :class="link.icon" class="ml-1"></i>
          {{ link.text }}
        </router-link>
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
import {errorNotification} from "@/parts/dialogs";

const authStore = useAuthStore()
const uiSettingsStore = useUISettingsStore()

const user = computed(() => {
    return authStore.user
})

const currentTheme = computed(() => {
    return uiSettingsStore.currentTheme
})

const showMobileMenu = ref(false)

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