import {trans} from "./i18n/i18n";

import HomeView from "./views/HomeView.vue";
import LoginView from "./views/LoginView.vue";

import {useAuthStore} from "./store/auth";
import Error404View from "./views/errors/Error404View.vue";
import Error403View from "./views/errors/Error403View.vue";
import Error500View from "./views/errors/Error500View.vue";
import {requestCancellation} from "./config/requestCancellation";

// Lazy-loaded views
const SsoView = () => import("./views/SsoView.vue");
const ServersView = () => import("./views/ServersView.vue");
const ProfileView = () => import("./views/ProfileView.vue");
const TokensView = () => import("./views/TokensView.vue");
const EmptyView = () => import("./views/EmptyView.vue");
const ServerIdView = () => import("./views/ServerIdView.vue");

// Lazy-loaded admin views (code-split for smaller initial bundle)
const AdminClientCertificatesView = () => import("./views/adminviews/AdminClientCertificatesView.vue");
const AdminDaemonTaskListView = () => import("./views/adminviews/AdminDaemonTaskListView.vue");
const AdminDaemonTaskOutputView = () => import("./views/adminviews/AdminDaemonTaskOutputView.vue");
const AdminGamesEdit = () => import("./views/adminviews/AdminGamesEdit.vue");
const AdminGamesImportView = () => import("./views/adminviews/AdminGamesImportView.vue");
const AdminGamesList = () => import("./views/adminviews/AdminGamesList.vue");
const AdminModEdit = () => import("./views/adminviews/AdminModEdit.vue");
const AdminNodesEditView = () => import("./views/adminviews/AdminNodesEditView.vue");
const AdminNodesView = () => import("./views/adminviews/AdminNodesView.vue");
const AdminPluginsView = () => import("./views/adminviews/AdminPluginsView.vue");
const AdminServersCreate = () => import("./views/adminviews/AdminServersCreate.vue");
const AdminServersEdit = () => import("./views/adminviews/AdminServersEdit.vue");
const AdminServersList = () => import("./views/adminviews/AdminServersList.vue");
const AdminUsersEditView = () => import("./views/adminviews/AdminUsersEditView.vue");
const AdminUsersView = () => import("./views/adminviews/AdminUsersView.vue");

const routes = [
    {
        path: '/403',
        name: 'error403',
        component: Error403View,
    },
    {
        path: '/404',
        name: 'error404',
        component: Error404View,
    },
    {
        path: '/500',
        name: 'error500',
        component: Error500View,
    },
    {
        path: '/login',
        name: 'login',
        component: LoginView,
    },
    {
        // Landing point for a single-use sign-in link issued by an external
        // system. Reachable without a session — that is the whole point.
        path: '/sso',
        name: 'sso',
        component: SsoView,
    },
    {
        path: '/',
        name: 'home',
        component: HomeView,
        alias: '/home'
    },
    {
        path: '/servers',
        name: 'servers',
        component: ServersView
    },
    {
        path: '/servers/:id',
        name: 'servers.control',
        component: ServerIdView
    },
    {
        path: '/admin/nodes',
        name: 'admin.nodes.index',
        component: AdminNodesView,
        alias: '/admin/dedicated_servers',
        meta: {
            title: trans('dedicated_servers.title_list'),
        },
    },
    {
        path: '/admin/nodes/:id',
        name: 'admin.nodes.view',
        alias: '/admin/dedicated_servers/:id',
        redirect: to => ({ name: 'admin.nodes.index', query: { node: String(to.params.id) } }),
    },
    {
        path: '/admin/nodes/:id/edit',
        name: 'admin.nodes.edit',
        component: AdminNodesEditView,
        alias: '/admin/dedicated_servers/:id/edit',
        meta: {
            title: trans('dedicated_servers.title_edit'),
        },
    },
    {
        path: '/admin/client_certificates',
        name: 'admin.client_certificates.index',
        component: AdminClientCertificatesView,
        meta: {
            title: trans('client_certificates.title_list'),
        },
    },
    {
        path: '/admin/servers',
        name: 'admin.servers.index',
        component: AdminServersList,
        meta: {
            title: trans('servers.title_servers_list'),
        }
    },
    {
        path: '/admin/servers/create',
        name: 'admin.servers.create',
        component: AdminServersCreate,
        meta: {
            title: trans('servers.title_create'),
        },
    },
    {
        path: '/admin/servers/:id/edit',
        name: 'admin.servers.edit',
        component: AdminServersEdit,
        meta: {
            title: trans('servers.title_edit'),
        },
    },
    {
        path: '/admin/games',
        name: 'admin.games.index',
        component: AdminGamesList,
        meta: {
            title: trans('games.title_games_list'),
        }
    },
    {
        path: '/admin/games/import',
        name: 'admin.games.import',
        component: AdminGamesImportView,
        meta: {
            title: trans('games.title_import'),
        }
    },
    {
        path: '/admin/games/:code',
        name: 'admin.games.edit',
        component: AdminGamesEdit,
        meta: {
            title: trans('games.title_edit'),
        }
    },
    {
        path: '/admin/games/:code/mods/:id/edit',
        name: 'admin.games.mods.edit',
        component: AdminModEdit,
        meta: {
            title: trans('games.title_edit_mod'),
        }
    },
    {
        path: '/admin/users',
        name: 'admin.users.index',
        component: AdminUsersView,
        meta: {
            title: trans('users.title_list')
        }
    },
    {
        path: '/admin/users/:id/edit',
        name: 'admin.users.edit',
        component: AdminUsersEditView,
        meta: {
            title: trans('users.title_edit')
        }
    },
    {
        path: '/admin/gdaemon_tasks',
        name: 'admin.gdaemon_tasks.index',
        component: AdminDaemonTaskListView,
        meta: {
            title: trans('gdaemon_tasks.title_list')
        }
    },
    {
        path: '/admin/gdaemon_tasks/:id',
        name: 'admin.gdaemon_tasks.output',
        component: AdminDaemonTaskOutputView,
        meta: {
            title: trans('gdaemon_tasks.title_view')
        }
    },
    {
        path: '/admin/plugins',
        name: 'admin.plugins.index',
        component: AdminPluginsView,
        meta: {
            title: trans('plugins.title_list')
        }
    },
    {
        path: '/profile',
        name: 'profile',
        component: ProfileView,
        meta: {
            title: trans('profile.title'),
        },
    },
    {
        path: '/tokens',
        name: 'tokens',
        component: TokensView,
        meta: {
            title: trans('tokens.tokens'),
        },
    },
    {
        path: '/report_bug',
        name: 'report_bug',
        component: EmptyView,
        meta: {
            title: trans('home.send_report')
        }
    },
    {
        path: '/plugins/:pluginId/:pathMatch(.*)*',
        name: 'plugin.pending',
        component: () => import('./views/PluginPendingView.vue'),
        meta: {
            requiresAuth: true,
            isPluginRoute: true,
        }
    },
    {
        path: '/:pathMatch(.*)*',
        redirect: '/404'
    },
]

const beforeEachRoute = (to, from) => {
    const authStore = useAuthStore()

    if (to.path !== from.path) {
        requestCancellation.setCurrentRoute(to.path)
        requestCancellation.createController(to.path)
    }

    const errorPages = ['error403', 'error404', 'error500']

    const guestPages = ['login', 'sso']

    if (!guestPages.includes(to.name) && !errorPages.includes(to.name) && !authStore.isAuthenticated) {
        return {name: 'login'}
    }

    if (to.name === 'login' && authStore.isAuthenticated) {
        return {name: 'home'}
    }

    if (to.meta.title) {
        document.title = to.meta.title;
    }
}


export {routes, beforeEachRoute}