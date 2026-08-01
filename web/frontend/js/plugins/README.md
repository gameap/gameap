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

## Available Slots

### `server-tabs`

**Status:** Active
**Location:** `views/ServerIdView.vue:221-236`

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
**Location:** `views/HomeView.vue:209`

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
**Location:** `views/HomeView.vue:53-72`

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

### `sidebar-sections`

**Status:** Defined (not integrated)
**Location:** Store only

Reserved for adding custom sections to the sidebar navigation.

---

### `admin-pages`

**Status:** Defined (not integrated)
**Location:** Store only

Reserved for adding custom pages to the admin area.

---

### `admin-user-info`

**Status:** Active
**Location:** `views/adminviews/AdminUsersView.vue`

Adds custom content to the user info modal in the admin users view. Content appears below the user details table.

#### Props Passed to Components

| Prop | Type | Description |
|------|------|-------------|
| `userId` | `number` | ID of the displayed user |
| `user` | `object` | Full user object from userStore |
| `pluginId` | `string` | ID of the plugin that registered the component |

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
  }
}
```

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
- `ServerTabProps` - Props for server-tabs components
- `DashboardWidgetProps` - Props for dashboard-widgets components
- `PermissionCheck` - Permission check types
- `SlotName` - Union of available slot names
