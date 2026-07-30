<template>
  <n-config-provider
      :locale="pageLanguage() === 'ru' ? ruRU : enUS"
      :theme="naiveTheme"
      :theme-overrides="naiveThemeOverrides"
  >
    <n-dialog-provider>
      <n-message-provider>
        <div v-if="user">
          <main-navbar></main-navbar>
          <status-notifier />
          <mfa-enforcement-modal />

          <div id="main-section" class="mt-16 mr-5 sm:flex">
            <div class="hidden sm:block flex-none">
              <main-sidebar></main-sidebar>
            </div>

            <div class="sm:flex-1 min-w-0">
              <div class="max-w-full">
                <div class="pt-3 pb-16 max-sm:pl-5 content">
                  <content-view></content-view>

                  <div v-if="!$route.name">
                  </div>

                </div>
              </div>
            </div>
          </div>
        </div>
        <div v-else>
          <guest-navbar></guest-navbar>

          <div id="main-section" class="mt-16 mr-5 sm:flex">
            <div class="sm:flex-1 sm:mr-5">
              <div class="max-w-full">
                <div class="pt-3 pb-5 content">
                  <content-view></content-view>
                </div>
              </div>
            </div>
          </div>
        </div>
      </n-message-provider>
    </n-dialog-provider>
  </n-config-provider>

</template>

<script setup>
import {computed, onMounted, provide, ref, watch} from "vue"
import {
  NConfigProvider,
  NDialogProvider,
  NMessageProvider,
  lightTheme,
  darkTheme,
  ruRU,
  enUS,
} from "naive-ui"
import {THEME_KEY} from "vue-echarts"
import MainNavbar from "./components/MainNavbar.vue"
import GuestNavbar from "./components/GuestNavbar.vue"
import MainSidebar from "./components/MainSidebar.vue"
import ContentView from "./components/ContentView.vue"
import StatusNotifier from "./components/StatusNotifier.vue"
import MfaEnforcementModal from "./components/blocks/MfaEnforcementModal.vue"
import {pageLanguage} from "./i18n/i18n"

import {useRoute, useRouter} from "vue-router"

import {useAuthStore} from "./store/auth"
import {useUISettingsStore} from "./store/uiSettings";
import {useNodeStore} from "./store/node"
import {useDaemonTaskStore} from "./store/daemonTask"
import {useGameStore} from "./store/game"
import {useServerStore} from "./store/server"
import {useUserStore} from "./store/user"

const route = useRoute()
const router = useRouter()

const authStore = useAuthStore()
const uiSettingsStore = useUISettingsStore()
const nodeStore = useNodeStore()
const daemonTaskStore = useDaemonTaskStore()
const gameStore = useGameStore()
const serverStore = useServerStore()
const userStore = useUserStore()

const user = computed(() => {
  return authStore.user
})

// naive-ui is CSS-in-JS and cannot consume the --gameap-* CSS variables
// directly, so its overrides are rebuilt from the resolved variable values on
// mount and on every theme switch. Fallbacks keep naive usable when
// /theme.css is missing (frontend-stub builds).
const FALLBACK = {
  primary: '#84cc16',
  primaryHover: '#65a30d',
  warning: '#fb923c',
  warningHover: '#f97316',
  danger: '#ef4444',
  dangerHover: '#dc2626',
  tableHeader: {light: '#f5f5f4', dark: '#44403c'},
  overlay: {light: '#ffffff', dark: '#292524'},
  raised: {light: '#ffffff', dark: '#292524'},
  textMuted: {light: '#78716c', dark: '#a8a29e'},
  tabAccent: {light: '#1c1917', dark: '#737373'},
  surfaceHover: '#262322',
}

function buildNaiveOverrides(mode) {
  const styles = getComputedStyle(document.documentElement)
  const v = (name, fallback) => {
    const value = styles.getPropertyValue(name).trim()

    return value || (typeof fallback === 'string' ? fallback : fallback[mode])
  }

  const overrides = {
    common: {
      primaryColor: v('--gameap-primary', FALLBACK.primary),
      primaryColorHover: v('--gameap-primary-hover', FALLBACK.primaryHover),
      primaryColorPressed: v('--gameap-primary-hover', FALLBACK.primaryHover),
      successColor: v('--gameap-success', FALLBACK.primary),
      successColorHover: v('--gameap-success-hover', FALLBACK.primaryHover),
      successColorPressed: v('--gameap-success-hover', FALLBACK.primaryHover),
      successColorSuppl: v('--gameap-success-hover', FALLBACK.primaryHover),
      warningColor: v('--gameap-warning', FALLBACK.warning),
      warningColorHover: v('--gameap-warning-hover', FALLBACK.warningHover),
      warningColorPressed: v('--gameap-warning-hover', FALLBACK.warningHover),
      warningColorSuppl: v('--gameap-warning-hover', FALLBACK.warningHover),
      errorColor: v('--gameap-danger', FALLBACK.danger),
      errorColorHover: v('--gameap-danger-hover', FALLBACK.dangerHover),
      errorColorPressed: v('--gameap-danger-hover', FALLBACK.dangerHover),
      errorColorSuppl: v('--gameap-danger-hover', FALLBACK.dangerHover),
      tableHeaderColor: v('--gameap-table-header', FALLBACK.tableHeader),
      modalColor: v('--gameap-surface-overlay', FALLBACK.overlay),
      cardColor: v('--gameap-surface-raised', FALLBACK.raised),
    },
    Tabs: {
      tabTextColorLine: v('--gameap-text-muted', FALLBACK.textMuted),
      tabTextColorActiveLine: v('--gameap-tab-accent', FALLBACK.tabAccent),
      tabTextColorHoverLine: v('--gameap-tab-accent', FALLBACK.tabAccent),
      barColor: v('--gameap-tab-accent', FALLBACK.tabAccent),
    },
  }

  if (mode === 'dark') {
    const rowHover = v('--gameap-surface-hover', FALLBACK.surfaceHover)
    overrides.DataTable = {
      tdColorHover: rowHover,
      tdColorStriped: rowHover,
      tdColorHoverModal: rowHover,
    }
  }

  return overrides
}

const naiveThemeOverrides = ref(buildNaiveOverrides(uiSettingsStore.currentTheme))

// getComputedStyle right after the classList mutation forces a synchronous
// style recalc, so the fresh variable values are already visible here.
function applyTheme(theme) {
  document.documentElement.classList.remove('dark', 'light')
  document.documentElement.classList.add(theme)
  naiveThemeOverrides.value = buildNaiveOverrides(theme)
}

onMounted(() => {
  applyTheme(uiSettingsStore.currentTheme)
})

watch(() => uiSettingsStore.currentTheme, (newTheme) => {
  applyTheme(newTheme)
})

const naiveTheme = computed(() => {
  return uiSettingsStore.currentTheme === 'dark' ? darkTheme : lightTheme
})

provide(THEME_KEY, computed(() => uiSettingsStore.currentTheme === 'dark' ? 'dark' : 'default'))

const onAnyStoreAction = ({
  name, // name of the action
  store, // store instance, same as `someStore`
  args, // array of parameters passed to the action
  after, // hook after the action returns or resolves
  onError, // hook if the action throws or rejects
}) => {
  onError((error) => {
    if (error.response && error.response.status) {
      switch (error.response.status) {
        case 401:
          authStore.logout().then(() => {
            window.location.href = '/'
          })
          break
        case 403:
          router.push({name: 'error403'})
          break
        case 404:
          router.push({name: 'error404'})
          break
      }
    }
  })
}

nodeStore.$onAction(onAnyStoreAction)
daemonTaskStore.$onAction(onAnyStoreAction)
gameStore.$onAction(onAnyStoreAction)
serverStore.$onAction(onAnyStoreAction)
userStore.$onAction(onAnyStoreAction)

</script>