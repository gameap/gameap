import type { PluginDefinition } from '@gameap/plugin-sdk';
import PluginPage from './components/PluginPage.vue';
import DashboardWidget from './components/DashboardWidget.vue';
import ServerTab from './components/ServerTab.vue';

export const myPlugin: PluginDefinition = {
    id: 'my-plugin',
    name: 'My Plugin',
    version: '1.0.0',
    apiVersion: '1.0',
    description: 'A sample GameAP plugin',
    author: 'Your Name',

    translations: {
        en: {
            'title': 'My Plugin Page',
            'welcome': 'Welcome to your plugin! This is the main plugin page.',
            'user_info': 'User Information',
            'logged_in_as': 'Logged in as',
            'admin': 'Admin',
            'yes': 'Yes',
            'no': 'No',
            'menu_item': 'My Plugin',
            'server_tab': 'My Tab',
            'dashboard_widget': 'My Widget',
        },
        ru: {
            'title': 'Страница плагина',
            'welcome': 'Добро пожаловать в ваш плагин! Это главная страница плагина.',
            'user_info': 'Информация о пользователе',
            'logged_in_as': 'Вы вошли как',
            'admin': 'Администратор',
            'yes': 'Да',
            'no': 'Нет',
            'menu_item': 'Мой плагин',
            'server_tab': 'Моя вкладка',
            'dashboard_widget': 'Мой виджет',
        },
    },

    routes: [
        {
            path: '/',
            name: 'index',
            component: PluginPage,
            meta: { title: 'My Plugin' },
        },
    ],

    menuItems: [
        {
            section: 'servers',
            icon: 'fas fa-puzzle-piece',
            text: '@:menu_item',
            route: { name: 'index' },
            order: 100,
        },
    ],

    homeButtons: [
        {
            name: '@:title',
            icon: 'fas fa-puzzle-piece',
            route: { name: 'index' },
            order: 100,
        },
    ],

    slots: {
        'dashboard-widgets': [
            {
                component: DashboardWidget,
                order: 50,
                label: '@:dashboard_widget',
            },
        ],
        'server-tabs': [
            {
                component: ServerTab,
                order: 100,
                label: '@:server_tab',
                icon: 'fas fa-puzzle-piece',
                name: 'my-tab',
            },
        ],
    },
};
