import {trans} from "../i18n/i18n";

const serversLinks = [
    {
        icon: 'play',
        text: trans('sidebar.servers'),
        route: {name: 'servers'},
    }
]

const adminLinks = [
    {
        icon: 'node',
        text: trans('sidebar.dedicated_servers'),
        route: {name: 'admin.nodes.index'}
    },
    {
        icon: 'server',
        text: trans('sidebar.game_servers'),
        route: {name: 'admin.servers.index'}
    },
    {
        icon: 'gamepad',
        text: trans('sidebar.games'),
        route: {name: 'admin.games.index'},
    },
    {
        icon: 'users',
        text: trans('sidebar.users'),
        route: {name: 'admin.users.index'}
    },
    {
        icon: 'plug',
        text: trans('plugins.plugins'),
        route: {name: 'admin.plugins.index'}
    }
]

export { serversLinks, adminLinks }
