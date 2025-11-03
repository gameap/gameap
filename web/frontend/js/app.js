/**
 * First we will load all of this project's JavaScript dependencies which
 * includes Vue and other libraries. It is a great starting point when
 * building robust, powerful web applications using Vue and Laravel.
 */

import '../sass/app.scss';

import {createApp, h} from "vue";
import {defineAsyncComponent} from 'vue'

import {createPinia} from 'pinia'

import {
    create,
    NAlert,
    NButton,
    NCard,
    NCheckbox,
    NCollapse,
    NCollapseItem,
    NConfigProvider,
    NDatePicker,
    NDataTable,
    NDialog,
    NDialogProvider,
    NEmpty,
    NInput,
    NInputNumber,
    NMessageProvider,
    NModal,
    NProgress,
    NRadio,
    NSelect,
    NTable,
    NTabs,
    NTabPane,
    NThemeEditor,
    NTooltip,
} from 'naive-ui'

import { createWebHistory, createRouter } from 'vue-router'

import './parts/form'
import {alert, confirmAction, confirm} from './parts/dialogs'

import {pluralize, trans, changeLanguage, getCurrentLanguage} from "./i18n/i18n";

import store from './legacy/store'
import { useAuthStore } from './store/auth'

import GBreadcrumbs from "./components/GBreadcrumbs.vue";
import GButton from "./components/GButton.vue";

import App from './App.vue';

import GuestNavbar from "./components/GuestNavbar.vue";
import MainNavbar from './components/MainNavbar.vue';
import MainSidebar from './components/MainSidebar.vue';

import ContentView from './components/ContentView.vue';

import KeyValueTable from "@/components/KeyValueTable.vue";
import GameapSelect from "@/components/input/GameapSelect.vue";
import TaskOutput from "@/components/TaskOutput.vue";

const InputTextList = defineAsyncComponent(() =>
    import('./components/input/InputTextList.vue')
)

const InputManyList = defineAsyncComponent(() =>
    import('./components/input/InputManyList.vue')
)

import fileManager from './filemanager';

const ServerStatus = defineAsyncComponent(() =>
    import('./views/servertabs/ServerStatus.vue')
)

const ServerConsole = defineAsyncComponent(() =>
    import('./views/servertabs/ServerConsole.vue')
)

const ServerTasks = defineAsyncComponent(() =>
    import('./views/servertabs/ServerTasks.vue')
)

const UserServerPrivileges = defineAsyncComponent(() =>
    import('./components/servers/UserServerPrivileges.vue' /* webpackChunkName: "components/user-server-privileges" */)
)

const GameModSelector = defineAsyncComponent(() =>
    import('./components/servers/GameModSelector.vue' /* webpackChunkName: "components/game-mod-selector" */)
)

const DsIpSelector = defineAsyncComponent(() =>
    import('./components/servers/DsIpSelector.vue' /* webpackChunkName: "components/game-mod-selector" */)
)

const SmartPortSelector = defineAsyncComponent(() =>
    import('./components/servers/SmartPortSelector.vue' /* webpackChunkName: "components/smart-port-selector" */)
)

const ServerSelector = defineAsyncComponent(() =>
    import('./components/servers/ServerSelector.vue' /* webpackChunkName: "components/server" */)
)

const SettingsParameters = defineAsyncComponent(() =>
    import('./components/SettingsParameters.vue' /* webpackChunkName: "components/settings" */)
)

// Blocks

const CreateNodeModal  = defineAsyncComponent(() =>
    import('./components/blocks/CreateNodeModal.vue' /* webpackChunkName: "components/blocks" */)
)

import {beforeEachRoute, routes} from "./routes";

const app = createApp(App)

// Global functions
app.config.globalProperties.pluralize = pluralize;
app.config.globalProperties.trans = trans;
app.config.globalProperties.changeLanguage = changeLanguage;
app.config.globalProperties.getCurrentLanguage = getCurrentLanguage;

const naive = create({
    components: [
        NAlert,
        NButton,
        NCard,
        NCheckbox,
        NCollapse,
        NCollapseItem,
        NConfigProvider,
        NDialog,
        NDataTable,
        NDatePicker,
        NDialogProvider,
        NEmpty,
        NInput,
        NInputNumber,
        NMessageProvider,
        NModal,
        NProgress,
        NRadio,
        NSelect,
        NTable,
        NTabs,
        NTabPane,
        NThemeEditor,
        NTooltip,
    ],
})

const pinia = createPinia()

app.use(store)

app.use(naive)

app.use(pinia)

const router = createRouter({
    history: createWebHistory(),
    routes,
})

router.beforeEach(beforeEachRoute)

app.use(router)

app.use(fileManager, {store: store})

// Initialize authentication from server session - MUST be after pinia is registered
const authStore = useAuthStore()

// Register global components BEFORE mounting
app.component('GBreadcrumbs', GBreadcrumbs);
app.component('GButton', GButton);
app.component('GuestNavbar', GuestNavbar);
app.component('MainNavbar', MainNavbar);
app.component('MainSidebar', MainSidebar);
app.component('ContentView', ContentView);
app.component('KeyValueTable', KeyValueTable);
app.component('DsIpSelector', DsIpSelector);
app.component('GameModSelector', GameModSelector);
app.component('GameapSelect', GameapSelect);
app.component('InputManyList', InputManyList);
app.component('InputTextList', InputTextList);
app.component('ServerConsole', ServerConsole);
app.component('ServerSelector', ServerSelector);
app.component('ServerStatus', ServerStatus);
app.component('ServerTasks', ServerTasks);
app.component('SettingsParameters', SettingsParameters);
app.component('SmartPortSelector', SmartPortSelector);
app.component('TaskOutput', TaskOutput);
app.component('UserServerPrivileges', UserServerPrivileges);
app.component('CreateNodeModal', CreateNodeModal);

// Global methods
app.config.globalProperties.alert = alert;
app.config.globalProperties.confirm = confirm;
app.config.globalProperties.confirmAction = confirmAction;

const meta = document.createElement('meta')
meta.name = 'naive-ui-style'
document.head.appendChild(meta)

// Initialize auth BEFORE mounting to prevent double mounting of components
// caused by v-if/v-else switching in App.vue when user profile loads
async function initializeApp() {
    try {
        await authStore.initializeAuth()
    } catch (error) {
        console.error('Failed to initialize authentication:', error)
    }

    try {
        app.mount("#app")
    } catch (error) {
        throw error;
    }
}

initializeApp()