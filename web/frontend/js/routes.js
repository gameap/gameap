import {trans} from "./i18n/i18n";

import EmptyView from "./views/EmptyView.vue";
import ServersView from "./views/ServersView.vue";
import HomeView from "./views/HomeView.vue";
import ProfileView from "./views/ProfileView.vue";
import TokensView from "./views/TokensView.vue";
import LoginView from "./views/LoginView.vue";

import {useAuthStore} from "./store/auth";
import Error404View from "./views/errors/Error404View.vue";
import Error403View from "./views/errors/Error403View.vue";
import Error500View from "./views/errors/Error500View.vue";
import {requestCancellation} from "./config/requestCancellation";

// Lazy-loaded view that contains FileManager (keeps filemanager out of main bundle)
const ServerIdView = () => import("./views/ServerIdView.vue");

// Lazy-loaded admin views (code-split for smaller initial bundle)
const AdminServersList = () => import("./views/adminviews/AdminServersList.vue");
const AdminServersCreate = () => import("./views/adminviews/AdminServersCreate.vue");
const AdminServersEdit = () => import("./views/adminviews/AdminServersEdit.vue");
const AdminNodesView = () => import("./views/adminviews/AdminNodesView.vue");
const AdminNodesCreateView = () => import("./views/adminviews/AdminNodesCreateView.vue");
const AdminNodesEditView = () => import("./views/adminviews/AdminNodesEditView.vue");
const AdminNodeShowView = () => import("./views/adminviews/AdminNodeShowView.vue");
const AdminClientCertificatesView = () => import("./views/adminviews/AdminClientCertificatesView.vue");
const AdminGamesList = () => import("./views/adminviews/AdminGamesList.vue");
const AdminGamesEdit = () => import("./views/adminviews/AdminGamesEdit.vue");
const AdminModEdit = () => import("./views/adminviews/AdminModEdit.vue");
const AdminUsersView = () => import("./views/adminviews/AdminUsersView.vue");
const AdminUsersEditView = () => import("./views/adminviews/AdminUsersEditView.vue");
const AdminDaemonTaskListView = () => import("./views/adminviews/AdminDaemonTaskListView.vue");
const AdminDaemonTaskOutputView = () => import("./views/adminviews/AdminDaemonTaskOutputView.vue");

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
        path: '/admin/nodes/create',
        name: 'admin.nodes.create',
        component: AdminNodesCreateView,
        alias: '/admin/dedicated_servers/create',
        meta: {
            title: trans('dedicated_servers.title_create'),
        },
    },
    {
        path: '/admin/nodes/:id',
        name: 'admin.nodes.view',
        component: AdminNodeShowView,
        alias: '/admin/dedicated_servers/:id',
        meta: {
            title: trans('dedicated_servers.title_view'),
        },
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

    if (to.name !== 'login' && !errorPages.includes(to.name) && !authStore.isAuthenticated) {
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