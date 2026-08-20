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
    fileEditors: [...],
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
| `fileEditors` | `PluginFileEditor[]` | No | Custom file editors for file manager |
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

## File Editors

Plugins can register custom file editors for the file manager. Editors provide specialized UIs for editing specific file types.

```typescript
import type { PluginDefinition } from '@gameap/plugin-sdk';
import ServerCfgEditor from './components/ServerCfgEditor.vue';
import IniEditor from './components/IniEditor.vue';
import HexEditor from './components/HexEditor.vue';

export const myPlugin: PluginDefinition = {
    id: 'my-plugin',
    name: 'My Plugin',
    version: '1.0.0',
    apiVersion: '1.0',
    fileEditors: [
        {
            id: 'server-cfg',
            name: 'Server Config Editor',
            component: ServerCfgEditor,
            match: {
                fileName: 'server.cfg',
                gameCode: 'cstrike'
            },
            icon: 'gear'
        },
        {
            id: 'ini-editor',
            name: 'INI Editor',
            component: IniEditor,
            match: {
                extensions: ['ini', 'cfg']
            }
        },
        {
            id: 'hex-editor',
            name: 'Hex Editor',
            component: HexEditor,
            match: {
                allFiles: true
            },
            contentType: 'binary',
            icon: 'memory'
        }
    ]
};
```

### File Editor Definition

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | Yes | Unique identifier within the plugin |
| `name` | `string` | Yes | Display name shown in context menu |
| `component` | `Component` | Yes | Vue component that renders the editor |
| `match` | `EditorMatchRules` | Yes | Rules for when this editor is available |
| `contentType` | `'text' \| 'binary' \| 'none'` | No | Content type (default: 'text') |
| `readOnly` | `boolean` | No | If true, editor is view-only (no save) |
| `icon` | `string` | No | Icon name from the `@gameap/ui` registry, e.g. `'file-archive'` |
| `contextMenuOnly` | `boolean` | No | Offer in the context menu only; never open on a double click |
| `menuLabel` | `string` | No | Caption of the context menu item instead of "Edit with …"; supports `@:key` |
| `checkPermission` | `PermissionCheck` | No | Hide the item unless the user holds these server abilities |

### Matching Rules

Editors match files based on these rules (all specified rules must match):

| Rule | Description | Example |
|------|-------------|---------|
| `allFiles` | Match all files (lowest specificity) | `true` |
| `fileName` | Exact file name match | `'server.cfg'` |
| `pathContains` | File path must contain string | `'amxmodx/configs/'` |
| `fullPath` | Exact full path match | `'/cstrike/server.cfg'` |
| `extensions` | Array of file extensions | `['ini', 'cfg', 'json']` |
| `fileNameRegexp` | Regex pattern for file name | `'^server.*\\.cfg$'` |
| `gameCode` | Match only for this game code | `'cstrike'` |
| `gameName` | Match only for this game name | `'Counter-Strike'` |

### Specificity and Default Editor

When multiple editors match a file:
- All matching editors are shown in the context menu
- The most specific editor is marked as "(default)" and opens on double-click
- Specificity order: `fullPath` > `pathContains` > `fileName` > `fileNameRegexp` > `extensions` > `allFiles`
- Game filters (`gameCode`/`gameName`) add to specificity score
- `allFiles: true` has the lowest specificity (score=1), useful for generic editors like hex viewers
- An editor with `contextMenuOnly: true` is never the default and never opens on a double click

### Editor Component Props

Editor components receive these props:

```typescript
interface FileEditorProps {
    content?: string | ArrayBuffer; // File content; absent for contentType: 'none'
    filePath: string;               // Full file path
    fileName: string;               // File name with extension
    extension: string;              // File extension (without dot)
    gameCode?: string;              // Current server's game code
    gameName?: string;              // Current server's game name
    fileSize?: number;              // Size in bytes, as the listing reported it
    fileMtime?: number;             // Modification time in unix seconds
    disk?: string;                  // File manager disk the file lives on
    pluginId: string;               // Plugin ID
}
```

The server the file belongs to is not a prop: read it with `useServerId()` /
`useServer()` from this SDK. Both throw when no plugin context is provided, so
wrap the call if your component can be mounted outside one.

### Editor Component Events

Emit these events from your editor component:

| Event | Payload | Description |
|-------|---------|-------------|
| `save` | `string \| ArrayBuffer` | Save the new content |
| `close` | - | Close the editor without saving |

### Example Editor Component

```vue
<template>
    <div class="my-editor">
        <textarea v-model="editedContent" rows="20" class="w-full"></textarea>
    </div>
</template>

<script setup>
import { ref, onMounted } from 'vue';

const props = defineProps({
    content: { type: String, required: true },
    filePath: { type: String, required: true },
    fileName: { type: String, required: true },
    extension: { type: String, required: true },
    gameCode: { type: String, default: null },
    gameName: { type: String, default: null },
    pluginId: { type: String, required: true }
});

const emit = defineEmits(['save', 'close']);

const editedContent = ref('');

onMounted(() => {
    editedContent.value = props.content;
});

function save() {
    emit('save', editedContent.value);
}

function close() {
    emit('close');
}

defineExpose({ save, close });
</script>
```

### Binary Content

For binary files (images, custom formats), set `contentType: 'binary'`:

```typescript
fileEditors: [
    {
        id: 'image-editor',
        name: 'Image Editor',
        component: ImageEditor,
        match: { extensions: ['png', 'jpg'] },
        contentType: 'binary'
    }
]
```

The `content` prop will be an `ArrayBuffer` instead of a string.

### Editors that load their own content

The modal downloads the file before it mounts the editor, and the file manager
refuses to open a file larger than 1 MB that way. An editor that does not need
the whole file — one that reports on it, compares it, or fetches only the part
it shows — declares `contentType: 'none'` instead: no `content` prop arrives,
the size cap does not apply, and the editor decides what to read.

```typescript
fileEditors: [
    {
        id: 'file-report',
        name: 'File report',
        component: FileReport,
        match: { allFiles: true },
        contentType: 'none',
        contextMenuOnly: true,
        readOnly: true
    }
]
```

`fileSize` and `fileMtime` come from the directory listing, so such an editor
can describe the file without downloading it. Saving still works: emit `save`
with the new content and the file manager writes it.

### Context-menu-only editors

The most specific matching editor is what a double click opens, ahead of the
file manager's own image, video, pdf and text handling. An editor registered
with `allFiles: true` therefore takes over every preview in the file manager —
which is right for a hex editor and wrong for anything that only inspects a
file. Set `contextMenuOnly: true` to stay out of the double click, and give
the item its own wording with `menuLabel`:

```typescript
{
    id: 'file-versions',
    name: '@:versions_title',
    menuLabel: '@:versions_menu',
    component: FileVersions,
    match: { allFiles: true },
    contentType: 'none',
    contextMenuOnly: true,
    checkPermission: {
        type: 'hasServerPermissions',
        permissions: ['plugin:myplugin:view']
    }
}
```

Without `menuLabel` the item reads "Edit with &lt;name&gt;". `checkPermission` takes
the same shape as a server tab's, and hides the item rather than letting the
plugin's own API answer 403.

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

### Using Translations in Plugin Routes

For components rendered as plugin routes (pages), use `usePluginTrans`:

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

### Using Translations in Slot Components

For slot components (server tabs, dashboard widgets, etc.), use `providePluginTrans` with the `pluginId` prop. This creates a translation context for the component and all its children:

```vue
<script setup>
import { providePluginTrans } from '@gameap/plugin-sdk';
import type { ServerTabProps } from '@gameap/plugin-sdk';

const props = defineProps<ServerTabProps>();

// providePluginTrans creates context for this component and its children
const { trans } = providePluginTrans(props.pluginId);
</script>

<template>
    <p>{{ trans('greeting', { name: 'World' }) }}</p>
</template>
```

**Why two different functions?**
- `usePluginTrans()` - Use in plugin routes/pages where the translation context is automatically provided
- `providePluginTrans(pluginId)` - Use in slot components (tabs, widgets) where you need to create the context manually

Child components of a slot component can still use `usePluginTrans()` - they will inherit the context from the parent.

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

Frontend bundles are embedded in WASM plugins and served by the GameAP backend. The plugin manager calls `GetFrontendBundle` during plugin initialization to retrieve the compiled JavaScript bundle and CSS styles.

### CSS Styles

Plugins can provide CSS styles that will be automatically loaded by GameAP. The CSS is returned alongside the JavaScript bundle in the `GetFrontendBundle` response.

When implementing your WASM plugin's `GetFrontendBundle` method, return styles in the response:

```protobuf
message GetFrontendBundleResponse {
  bytes bundle = 1;      // JavaScript bundle
  bool has_bundle = 2;   // Whether plugin has JS bundle
  bytes styles = 3;      // CSS styles
  bool has_styles = 4;   // Whether plugin has CSS styles
}
```

CSS from all plugins is combined and served at `/plugins.css`, which is automatically loaded before plugin JavaScript.

## Theming

The panel's colors are driven by CSS custom properties with the `--gameap-`
prefix, defined in `/theme.css` (source: `@gameap/ui/theme.css`). The full
variable table lives in the `@gameap/ui` README ("Theming" section).

### Load order guarantees

1. `/theme.css` — `<link>` in the document head (variable definitions)
2. Panel CSS — `main-*.css`
3. Plugin CSS — `<style id="gameap-plugin-styles">`, injected **before** the
   app mounts
4. Lazy route chunks (e.g. the file manager) — injected whenever their route
   loads, i.e. **after** plugin CSS

Consequence: plugin overrides of `--gameap-*` variables always win, while
selector-level overrides of panel classes are unreliable (a lazy chunk loaded
later out-cascades them at equal specificity). **Override variables, not
selectors.**

### Re-theming the panel from a plugin

Ship regular CSS in your plugin bundle:

```css
:root {
    --gameap-primary: #e11d48;
    --gameap-primary-hover: #be123c;
}

html.dark {
    --gameap-surface: #1e1b4b;
}
```

Dark-mode values must be overridden under `html.dark` (the panel toggles the
`dark` class on `<html>`); a plain `:root` declaration loses to the panel's
own `html.dark` rules.

Limitations:

- `/plugins.css` is served behind authentication, so plugin theme overrides do
  not apply to the login screen (the `/theme.css` defaults do).
- naive-ui components are CSS-in-JS. The panel derives its naive-ui theme from
  the variables on load and on every light/dark switch, so variable overrides
  reach naive components too — but only the variables the panel maps:
  `--gameap-{primary,success,warning,danger}[-hover]`, `--gameap-table-header`,
  `--gameap-surface-overlay`, `--gameap-surface-raised`, `--gameap-surface-hover`,
  `--gameap-text-muted`, `--gameap-tab-accent`.

### Using panel colors in plugin UI

In plugin templates, prefer the safelisted semantic utility classes
(`bg-surface`, `text-muted`, `bg-primary`, `text-danger`, `border-strong`, …;
full list in the `@gameap/ui` README) — they follow the active theme and need
no `dark:` variants. In plugin CSS, use `var(--gameap-*)` directly. Other
Tailwind classes are only available if the panel's own markup happens to use
them, so do not rely on arbitrary utilities.

### Testing themes

Use the `@gameap/debug` harness:

```js
window.gameapDebug.loadPlugin(
    'export const themeTest = {id: "theme-test", name: "Theme Test", version: "1.0.0"}',
    ':root { --gameap-primary: #e11d48; }\nhtml.dark { --gameap-surface: #1e1b4b; }'
)
```

## Development

```bash
# Watch mode for development
npm run dev

# Production build
npm run build
```

## Template

A complete plugin template is available in `templates/plugin-template/`. Copy this directory as a starting point for your plugin.
