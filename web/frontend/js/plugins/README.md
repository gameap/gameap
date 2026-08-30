# Plugin Slots System

This document describes the plugin slots system in GameAP, which allows plugins to inject components into
predefined locations throughout the application.

## Overview

The plugin slot system provides extension points where plugins can register Vue components. When the application
renders a slot, all registered components for that slot are displayed in order.

### Core Components

| File | Purpose |
|------|---------|
| `store/plugins/index.js` | Pinia store managing slot registration and component retrieval |
| `plugins/components/PluginSlot.vue` | Generic component for rendering slot contents |
| `plugins/loader.js` | Plugin loading and registration logic |
| `plugins/context.js` | Plugin context provider (route, server, user data) |
| `plugins/permissions.js` | Shared `checkPermission` / `checkGame` evaluation |

## Available Slots

| Slot | Where it renders |
|------|------------------|
| `server-tabs` | An extra tab on the server page |
| `server-control-buttons` | The Start / Stop / Restart button row on the server page |
| `server-control-blocks` | The server page Control tab, between the status card and the console |
| `servers-list-actions` | The commands column of the server list |
| `dashboard-widgets` | The home page, below the buttons |
| `home-buttons` | The home page button row |
| `navbar-items` | The top navigation bar, left of the theme switch |
| `sidebar-sections` | The sidebar, below the menu sections |
| `global-banners` | Above the content of every page |
| `admin-pages` | Above the content of `/admin/*` pages, administrators only |
| `profile-info-rows` | Extra rows in the profile table |
| `profile-blocks` | The profile page, below the two-factor card |
| `admin-user-info-above` | The admin user modal, above the details table |
| `admin-user-info-rows` | Extra rows in the admin user modal table |
| `admin-user-info` | The admin user modal, below the details table |
| `admin-user-edit-blocks` | The user edit page, below the Servers card |
| `admin-node-edit-blocks` | The node edit page, at the end of the Main tab |
| `admin-server-edit-blocks` | The server edit page, below the last card |
| `admin-game-edit-blocks` | The game edit page, below the tabs |
| `admin-mod-edit-blocks` | The mod edit page, below the tabs |

### `server-tabs`

**Status:** Active
**Location:** `views/ServerIdView.vue` (the `n-tab-pane v-for="tab in pluginTabs"` block)

Adds custom tabs to the server detail page alongside Console, Files, Schedules, etc.

#### Props Passed to Components

| Prop | Type | Description |
|------|------|-------------|
| `serverId` | `number` | Current server ID |
| `server` | `ServerData` | Full server object |
| `pluginId` | `string` | ID of the plugin that registered the component |

#### Permission Checking

Supports `hasServerPermissions` permission check:

```javascript
{
  component: MyTabComponent,
  label: 'My Tab',
  icon: 'puzzle-piece',
  checkPermission: {
    type: 'hasServerPermissions',
    permissions: ['console-view', 'rcon-console']
  }
}
```

The tab is only shown if the user has all specified permissions for the current server.

#### Game Matching

Supports `checkGame` to show the tab only for servers running specific games.
The tab is shown when the server's game matches at least one listed engine
(case-insensitive) or game code. Both conditions can be combined; while the
server data is still loading, the tab stays hidden.

```javascript
{
  component: MyTabComponent,
  label: 'My Tab',
  icon: 'puzzle-piece',
  checkGame: {
    engines: ['GoldSource'],       // matches game.engine
    codes: ['cstrike', 'valve']    // matches game.code
  }
}
```

`checkGame` and `checkPermission` are independent: when both are set, the tab
is shown only if both checks pass.

#### Usage Example

```javascript
export const MyPlugin = {
  id: 'my-plugin',
  name: 'My Plugin',
  version: '1.0.0',
  apiVersion: '1.0',
  slots: {
    'server-tabs': [{
      component: ServerStatsTab,
      label: '@:tabs.stats',  // Translation reference
      icon: 'metrics',
      order: 10,
      name: 'stats',
      checkPermission: {
        type: 'hasServerPermissions',
        permissions: ['console-view']
      }
    }]
  }
}
```

---

### `dashboard-widgets`

**Status:** Active
**Location:** `views/HomeView.vue` (below the home buttons)

Adds widgets to the home/dashboard page below the main navigation buttons.

#### Props Passed to Components

| Prop | Type | Description |
|------|------|-------------|
| `isAdmin` | `boolean` | Whether the current user is an admin |
| `pluginId` | `string` | ID of the plugin that registered the component |

#### Usage Example

```javascript
export const MyPlugin = {
  id: 'my-plugin',
  name: 'My Plugin',
  version: '1.0.0',
  apiVersion: '1.0',
  slots: {
    'dashboard-widgets': [{
      component: SystemStatusWidget,
      order: 5
    }]
  }
}
```

---

### `home-buttons`

**Status:** Active
**Location:** `views/HomeView.vue` (the home buttons row)

Adds navigation buttons to the home page next to the Servers and Nodes buttons.

#### Special Handling

This slot has special handling compared to other slots:
- If a `component` is provided, it renders the custom component
- If no component is provided, renders a default `GButton` with icon and label

#### Props Passed to Components

| Prop | Type | Description |
|------|------|-------------|
| `route` | `object` | Route object for navigation (auto-generated if not specified) |
| `pluginId` | `string` | ID of the plugin that registered the component |

#### Registration Methods

**Method 1: Via `homeButtons` in plugin definition**

```javascript
export const MyPlugin = {
  id: 'my-plugin',
  name: 'My Plugin',
  version: '1.0.0',
  apiVersion: '1.0',
  homeButtons: [{
    name: 'Analytics',
    icon: 'metrics',
    route: { name: 'index' },  // Becomes plugin.my-plugin.index
    order: 10
  }]
}
```

**Method 2: Custom component via slots**

```javascript
export const MyPlugin = {
  id: 'my-plugin',
  name: 'My Plugin',
  version: '1.0.0',
  apiVersion: '1.0',
  slots: {
    'home-buttons': [{
      component: CustomHomeButton,
      order: 10
    }]
  }
}
```

---

### `server-control-buttons`

**Status:** Active
**Location:** `views/ServerIdView.vue` (inside `div#serverControl`)

Adds buttons to the Start / Stop / Restart / Update row on the server page, for plugin
commands that deserve to sit next to the built-in ones (backup, wipe, map change).

This slot **evaluates `checkPermission` and `checkGame`** - a component that declares them is
rendered only for viewers who actually hold the abilities, and only for matching games.

#### Props Passed to Components

| Prop | Type | Description |
|------|------|-------------|
| `serverId` | `number` | ID of the displayed server |
| `server` | `object` | Full server object |
| `abilities` | `object` | Map of the viewer's abilities for this server |
| `pluginId` | `string` | ID of the plugin that registered the component |

```javascript
slots: {
  'server-control-buttons': [{
    component: BackupButton,
    order: 10,
    checkPermission: { type: 'hasServerPermissions', permissions: ['game-server-update'] }
  }]
}
```

---

### `server-control-blocks`

**Status:** Active
**Location:** `views/ServerIdView.vue` (Control tab, between the status card and the console)

Adds full-width blocks to the server Control tab for content that should be visible without
switching to a plugin tab. Same props and same `checkPermission` / `checkGame` handling as
`server-control-buttons`.

---

### `servers-list-actions`

**Status:** Active
**Location:** `views/servertabs/ServerMainList.vue` (commands column)

Adds elements to the commands column of the server list. The list builds its columns with
render functions, so components are instantiated directly rather than through `PluginSlot`;
`checkPermission` and `checkGame` are evaluated per row.

#### Props Passed to Components

| Prop | Type | Description |
|------|------|-------------|
| `serverId` | `number` | ID of the server in this row |
| `server` | `object` | Full server object for this row |
| `pluginId` | `string` | ID of the plugin that registered the component |

Nothing is rendered for disabled or blocked servers.

---

### `navbar-items`

**Status:** Active
**Location:** `components/MainNavbar.vue` (left of the theme switch)

Adds elements to the top navigation bar - counters, notification bells, quick actions.
Keep them small: the bar is 4rem tall and shared with the help and profile dropdowns.

#### Props Passed to Components

| Prop | Type | Description |
|------|------|-------------|
| `isAdmin` | `boolean` | Whether the viewer is an administrator |
| `pluginId` | `string` | ID of the plugin that registered the component |

---

### `sidebar-sections`

**Status:** Active
**Location:** `components/MainSidebar.vue` (both the collapsed and the expanded sidebar)

Adds custom sections to the sidebar, below the built-in menus. The slot is rendered twice -
once per sidebar variant - so use `minimized` to render an icon-only version when the
sidebar is collapsed to 4rem.

#### Props Passed to Components

| Prop | Type | Description |
|------|------|-------------|
| `minimized` | `boolean` | Whether the sidebar is collapsed to icons |
| `isAdmin` | `boolean` | Whether the viewer is an administrator |
| `pluginId` | `string` | ID of the plugin that registered the component |

For plain navigation links prefer `menuItems` in the plugin definition - it already renders
into the Control, Admin and custom sidebar sections.

---

### `global-banners`

**Status:** Active
**Location:** `components/ContentView.vue` (above the router view)

Adds a strip above the content of every page - licence warnings, "backup overdue" notices
and similar. `components/StatusBanner.vue` is the built-in component for that look.

#### Props Passed to Components

| Prop | Type | Description |
|------|------|-------------|
| `routeName` | `string \| null` | Name of the current route |
| `isAdmin` | `boolean` | Whether the viewer is an administrator |
| `pluginId` | `string` | ID of the plugin that registered the component |

---

### `admin-pages`

**Status:** Active
**Location:** `components/ContentView.vue` (above the router view, `/admin/*` only)

Same position as `global-banners`, but scoped: rendered only on administrative pages and
only for administrators. Use it for content that belongs to the admin section as a whole.

Full plugin pages do not need this slot - `routes` and `menuItems` in the plugin definition
already deliver them under `/plugins/{pluginId}/...` with an entry in the admin menu.

Props are the same as `global-banners`.

---

### `profile-info-rows`

**Status:** Active
**Location:** `views/ProfileView.vue` (inside the profile table body)

Adds rows to the profile details table.

> **The component's root element MUST be `<tr>`** with two `<td>` cells, matching the
> built-in rows. It is rendered directly inside `<tbody>`; any other root tag produces an
> invalid table.

```vue
<template>
  <tr>
    <td><strong>{{ trans('@:my.label') }}:</strong></td>
    <td>{{ value }}</td>
  </tr>
</template>
```

#### Props Passed to Components

| Prop | Type | Description |
|------|------|-------------|
| `userId` | `number` | ID of the current user |
| `user` | `object` | Full user object |
| `pluginId` | `string` | ID of the plugin that registered the component |

---

### `profile-blocks`

**Status:** Active
**Location:** `views/ProfileView.vue` (below the two-factor card)

Adds blocks to the profile page. Same props as `profile-info-rows`. Components are rendered
as-is, so wrap the content in a `GCard` to match the page.

---

### `admin-user-info-above`

**Status:** Active
**Location:** `views/adminviews/AdminUsersView.vue` (user modal, above the details table)

Adds content to the top of the user info modal in the admin users view.

#### Props Passed to Components

| Prop | Type | Description |
|------|------|-------------|
| `userId` | `number` | ID of the displayed user |
| `user` | `object` | Full user object from userStore |
| `pluginId` | `string` | ID of the plugin that registered the component |

---

### `admin-user-info-rows`

**Status:** Active
**Location:** `views/adminviews/AdminUsersView.vue` (user modal, inside the table body)

Adds rows to the user details table in the modal. Same props as `admin-user-info-above`.

> **The component's root element MUST be `<tr>`** with two `<td>` cells - see
> `profile-info-rows` for the markup.

---

### `admin-user-info`

**Status:** Active
**Location:** `views/adminviews/AdminUsersView.vue` (user modal, below the details table)

Adds content below the user details table. Same props as `admin-user-info-above`.

#### Usage Example

```javascript
export const MyPlugin = {
  id: 'my-plugin',
  name: 'My Plugin',
  version: '1.0.0',
  apiVersion: '1.0',
  slots: {
    'admin-user-info': [{
      component: UserActivityWidget,
      order: 10
    }]
  }
}
```

---

### `admin-user-edit-blocks`

**Status:** Active
**Location:** `views/adminviews/forms/UpdateUserForm.vue` (below the Servers card)

Adds blocks to the user edit page, between the Servers card and the Save button.

#### Props Passed to Components

| Prop | Type | Description |
|------|------|-------------|
| `userId` | `number` | ID of the edited user |
| `user` | `object` | Saved user object from userStore |
| `form` | `object` | Read-only snapshot of the **unsaved** form: `login`, `email`, `name`, `roles`, `servers` |
| `pluginId` | `string` | ID of the plugin that registered the component |

`form` never carries the password fields. The panel does not save plugin data together with
the form - a plugin persists its own state with its own API calls.

---

### `admin-node-edit-blocks`, `admin-server-edit-blocks`, `admin-game-edit-blocks`, `admin-mod-edit-blocks`

**Status:** Active
**Location:** `views/adminviews/forms/UpdateNodeForm.vue` (end of the Main tab),
`views/adminviews/AdminServersEdit.vue`, `views/adminviews/forms/UpdateGameForm.vue`,
`views/adminviews/forms/UpdateModForm.vue` (below the tabs)

Add blocks to the edit pages of the remaining entities, following the same rules as
`admin-user-edit-blocks`.

| Slot | Identity prop | `form` snapshot |
|------|---------------|-----------------|
| `admin-node-edit-blocks` | `nodeId` | `name`, `enabled`, `os`, `location`, `provider`, `workPath`, `steamcmdPath`, `ip` - daemon credentials, certificates and control scripts are never exposed |
| `admin-server-edit-blocks` | `serverId`, plus the saved `server` object | everything but the RCON password |
| `admin-game-edit-blocks` | `gameCode` | the whole form |
| `admin-mod-edit-blocks` | `modId` | the whole form |

---

### Rendering blocks that match the panel

Block slots render components as-is, without wrapping them in a card, so a plugin that wants
to look like a built-in section renders its own:

```vue
<template>
  <div class="flex-wrap md:grid mt-2">
    <div class="md:w-full">
      <n-card
          :title="title"
          size="small"
          class="mb-3"
          header-class="g-card-header"
          :segmented="{ content: true, footer: 'soft' }"
      >
        <!-- ... -->
      </n-card>
    </div>
  </div>
</template>
```

---

## Slot Component Registration

### Registration Options

When registering a component to a slot, the following options are available:

```javascript
{
  component: VueComponent,     // Required: Vue component to render
  order: 0,                    // Sort order (lower = first)
  label: 'Tab Label',          // Display label (supports @:key translation refs)
  icon: 'metrics',             // Icon name from the @gameap/ui icon registry
  name: 'unique-name',         // Unique identifier within the slot
  props: {},                   // Default props to pass to the component
  checkPermission: {           // Optional permission check
    type: 'hasServerPermissions',
    permissions: ['perm1', 'perm2']
  },
  checkGame: {                 // Optional game match
    engines: ['source'],
    codes: ['cstrike']
  }
}
```

`checkPermission` and `checkGame` are evaluated only by the slots that know the server
context: `server-tabs`, `server-control-buttons`, `server-control-blocks` and
`servers-list-actions` (plus the file editors). Everywhere else they are ignored and the
component is rendered regardless - do not rely on them to hide sensitive content in, say,
`admin-user-info`.

### How PluginSlot Component Works

The `PluginSlot.vue` component:

1. Retrieves registered components for the slot name from the plugins store
2. Sorts components by their `order` property (ascending)
3. Renders each component with merged props:
   - Component's registered `props`
   - Slot's `context` prop (passed by the parent view)
   - `pluginId` for identification

```vue
<PluginSlot name="dashboard-widgets" :context="{ isAdmin }" />
```

A host that knows the server context opts into the permission checks by passing
`check-context`; without it every registered component is rendered:

```vue
<PluginSlot
    name="server-control-buttons"
    :context="serverSlotContext"
    :check-context="{ abilities, game }"
/>
```

The check itself lives in `plugins/permissions.js` (`matchesPermission`, `matchesGame`,
`filterSlotComponents`) and is shared with the server tabs.

### Component Lifecycle

1. **Plugin Loading:** `loadPlugins()` fetches and evaluates `plugins.js`
2. **Registration:** `registerPluginDefinition()` processes each plugin's `slots` config
3. **Store Update:** Components are added to `slots[slotName]` array via `registerSlotComponent()`
4. **Rendering:** Views use `PluginSlot` component or direct iteration over `getSlotComponents()`

---

## Plugin Context

All slot components have access to a plugin context via `usePluginContext()`:

```javascript
import { usePluginContext } from '@/plugins'

const context = usePluginContext()

// Available data:
context.route     // ComputedRef<PluginRouteInfo>
context.server    // ComputedRef<{ id, data, abilities }>
context.user      // ComputedRef<UserData>
context.stores    // Direct access to auth, server, plugins stores
```

---

## Translations

Slot labels support translation references with the `@:key` prefix:

```javascript
{
  label: '@:tabs.serverStats'
}
```

The system resolves translations from the plugin's `translations` object based on current locale.

---

## TypeScript Types

See `web/plugin-sdk/src/types.ts` for complete type definitions:

- `PluginSlotComponent` - Slot component registration
- `ServerTabProps` - Props for `server-tabs` components
- `DashboardWidgetProps` - Props for `dashboard-widgets` components
- `ServerControlProps` - Props for `server-control-buttons` / `server-control-blocks`
- `ServersListActionProps` - Props for `servers-list-actions`
- `ChromeSlotProps` - Props for `navbar-items`, `global-banners`, `admin-pages`
- `SidebarSectionProps` - Props for `sidebar-sections`
- `ProfileSlotProps` - Props for `profile-info-rows` / `profile-blocks`
- `AdminUserInfoProps` - Props for the admin user modal slots
- `AdminUserEditBlockProps`, `AdminNodeEditBlockProps`, `AdminServerEditBlockProps`,
  `AdminGameEditBlockProps`, `AdminModEditBlockProps` - Props for the entity edit blocks
- `PermissionCheck`, `GameCheck` - Condition types
- `SlotName` - Union of available slot names
