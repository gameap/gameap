# GameAP Plugin SDK

SDK for developing GameAP frontend plugins. 
This package provides types, utilities, and build configuration for creating Vue-based plugins 
that integrate with the GameAP.

## Installation

```bash
npm install @gameap/plugin-sdk
```

## Quick Start

1. Copy the template from `templates/plugin-template/` to your plugin directory
2. Update `package.json` with your plugin details
3. Implement your plugin in `src/index.ts`
4. Build with `npm run build`

## Plugin Structure

A plugin must export a `PluginDefinition` object:

```typescript
import type { PluginDefinition } from '@gameap/plugin-sdk';
import MyComponent from './components/MyComponent.vue';

export const myPlugin: PluginDefinition = {
    id: 'my-plugin',
    name: 'My Plugin',
    version: '1.0.0',
    apiVersion: '1.0',
    description: 'A sample plugin',
    author: 'Your Name',
    routes: [...],
    menuItems: [...],
    slots: {...},
    homeButtons: [...],
};
```

## Plugin Definition Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | Yes | Unique plugin identifier (e.g., 'my-plugin') |
| `name` | `string` | Yes | Human-readable plugin name |
| `version` | `string` | Yes | Semantic version (e.g., '1.0.0') |
| `apiVersion` | `'1.0'` | Yes | GameAP plugin API version |
| `description` | `string` | No | Plugin description |
| `author` | `string` | No | Plugin author |
| `routes` | `PluginRoute[]` | No | Plugin routes to register |
| `menuItems` | `PluginMenuItem[]` | No | Sidebar menu items |
| `slots` | `Record<SlotName, PluginSlotComponent[]>` | No | Slot components |
| `homeButtons` | `PluginHomeButton[]` | No | Home page buttons |
| `translations` | `Record<string, Record<string, string>>` | No | Plugin translations by language |
| `onInit` | `() => void \| Promise<void>` | No | Initialization hook |
| `onDestroy` | `() => void \| Promise<void>` | No | Cleanup hook |

## Routes

Plugin routes are registered under `/plugins/{pluginId}/`:

```typescript
routes: [
    {
        path: '/',              // Resolves to /plugins/my-plugin/
        name: 'index',          // Route name: plugin.my-plugin.index
        component: MyPage,
        meta: {
            title: 'My Page',
            requiresAuth: true,
            requiresAdmin: false,
        },
        children: [...],        // Nested routes
    },
],
```

## Menu Items

Add items to the sidebar:

```typescript
menuItems: [
    {
        section: 'servers',     // 'servers' | 'admin' | 'custom'
        icon: 'fas fa-puzzle-piece',
        text: 'My Plugin',
        route: { name: 'index' },
        order: 100,
        adminOnly: false,
    },
],
```

## Slots

Inject components into predefined slots:

```typescript
slots: {
    'server-tabs': [
        {
            component: ServerTab,
            order: 100,
            label: 'My Tab',
            icon: 'fas fa-puzzle-piece',
            name: 'my-tab',
        },
    ],
    'dashboard-widgets': [
        {
            component: DashboardWidget,
            order: 50,
            label: 'My Widget',
        },
    ],
},
```

### Available Slots

| Slot Name | Description | Props |
|-----------|-------------|-------|
| `server-tabs` | Tabs on server detail pages | `ServerTabProps` |
| `dashboard-widgets` | Widgets on the dashboard | `DashboardWidgetProps` |
| `sidebar-sections` | Sections in the sidebar | - |
| `admin-pages` | Admin page components | - |

## Home Buttons

Add buttons to the home page:

```typescript
homeButtons: [
    {
        name: 'My Plugin',
        icon: 'fas fa-puzzle-piece',
        route: { name: 'index' },
        order: 100,
    },
],
```

## Internationalization (i18n)

Plugins can provide translations for multiple languages:

```typescript
export const myPlugin: PluginDefinition = {
    id: 'my-plugin',
    name: 'My Plugin',
    version: '1.0.0',
    apiVersion: '1.0',
    translations: {
        en: {
            'greeting': 'Hello, :name!',
            'status.active': 'Active',
            'status.inactive': 'Inactive'
        },
        ru: {
            'greeting': 'Привет, :name!',
            'status.active': 'Активен',
            'status.inactive': 'Неактивен'
        }
    },
    routes: [...]
};
```

Use translations in components with `usePluginTrans`:

```vue
<script setup>
import { usePluginTrans } from '@gameap/plugin-sdk';

const { trans, locale } = usePluginTrans();
</script>

<template>
    <p>{{ trans('greeting', { name: 'World' }) }}</p>
    <span>{{ trans('status.active') }}</span>
    <p>Current locale: {{ locale }}</p>
</template>
```

The `trans` function:
- Returns the translated string for the current locale
- Falls back to English (`en`) if the current locale is not available
- Returns the key itself if no translation is found
- Supports parameter substitution using `:paramName` syntax

### Translating Menu Items and Slot Labels

You can use the `@:key` syntax to reference translations in menu items, slots, and home buttons:

```typescript
export const myPlugin: PluginDefinition = {
    translations: {
        en: {
            'menu_item': 'My Plugin',
            'server_tab': 'My Tab',
            'home_button': 'My Plugin'
        },
        ru: {
            'menu_item': 'Мой плагин',
            'server_tab': 'Моя вкладка',
            'home_button': 'Мой плагин'
        }
    },
    menuItems: [{
        text: '@:menu_item',  // Will be translated
        // ...
    }],
    slots: {
        'server-tabs': [{
            label: '@:server_tab',  // Will be translated
            // ...
        }]
    },
    homeButtons: [{
        name: '@:home_button',  // Will be translated
        // ...
    }]
};
```

The `@:key` syntax:
- Prefix any text value with `@:` followed by the translation key
- The value will be automatically resolved to the current locale's translation
- Falls back to English if the current locale is not available
- Returns the key itself if no translation is found

## Context Hooks

Access GameAP context in your components:

```vue
<script setup>
import {
    usePluginContext,
    useServer,
    useServerId,
    useServerAbilities,
    useCurrentUser,
    useIsAdmin,
    useIsAuthenticated,
    usePluginRoute,
    usePluginId,
} from '@gameap/plugin-sdk';

// Full context access
const ctx = usePluginContext();

// Server data (on server pages)
const server = useServer();           // ServerData | null
const serverId = useServerId();       // number | null
const abilities = useServerAbilities(); // string[]

// User data
const user = useCurrentUser();        // UserData
const isAdmin = useIsAdmin();         // boolean
const isAuthenticated = useIsAuthenticated(); // boolean

// Route info
const route = usePluginRoute();       // PluginRouteInfo
const pluginId = usePluginId();       // string | null
</script>
```

## Component Props

### ServerTabProps

Passed to components in the `server-tabs` slot:

```typescript
interface ServerTabProps {
    serverId: number;
    server: ServerData;
    pluginId: string;
}
```

### DashboardWidgetProps

Passed to components in the `dashboard-widgets` slot:

```typescript
interface DashboardWidgetProps {
    isAdmin: boolean;
    pluginId: string;
}
```

## Data Types

### ServerData

```typescript
interface ServerData {
    id: number;
    uuid: string;
    name: string;
    game_id: string;
    game_mod_id: number;
    ip: string;
    port: number;
    query_port: number;
    rcon_port: number;
    enabled: boolean;
    installed: boolean;
    blocked: boolean;
    start_command: string;
    dir: string;
    process_active: boolean;
    last_process_check: string;
}
```

### UserData

```typescript
interface UserData {
    id: number;
    login: string;
    name: string;
    roles: string[];
    isAdmin: boolean;
    isAuthenticated: boolean;
}
```

## Build Configuration

Use the provided Vite configuration helper:

```javascript
// vite.config.js
import { createPluginConfig } from '@gameap/plugin-sdk/vite';

export default createPluginConfig({
    entry: 'src/index.ts',
    name: 'plugin',
    outDir: 'dist',
});
```

## Integration with WASM Plugins

Frontend bundles are embedded in WASM plugins and served by the GameAP backend. The plugin manager calls `GetFrontendBundle` during plugin initialization to retrieve the compiled JavaScript bundle.

## Development

```bash
# Watch mode for development
npm run dev

# Production build
npm run build
```

## Template

A complete plugin template is available in `templates/plugin-template/`. Copy this directory as a starting point for your plugin.
