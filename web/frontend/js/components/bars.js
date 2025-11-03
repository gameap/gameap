import {trans} from "../i18n/i18n";

const serversLinks = [
    {
        icon: 'fas fa-play',
        text: trans('sidebar.servers'),
        route: {name: 'servers'},
    }
]

const adminLinks = [
    {
        icon: 'fas fa-hdd',
        text: trans('sidebar.dedicated_servers'),
        route: {name: 'admin.nodes.index'}
    },
    {
        icon: 'fas fa-server',
        text: trans('sidebar.game_servers'),
        route: {name: 'admin.servers.index'}
    },
    {
        icon: 'fas fa-gamepad',
        text: trans('sidebar.games'),
        route: {name: 'admin.games.index'},
    },
    {
        icon: 'fas fa-tasks',
        text: trans('sidebar.gdaemon_tasks'),
        route: {name: 'admin.gdaemon_tasks.index'},
    },
    {
        icon: 'fas fa-users',
        text: trans('sidebar.users'),
        route: {name: 'admin.users.index'}
    },
]

export { serversLinks, adminLinks }