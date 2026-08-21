# @gameap/ui

A Vue 3 component library providing GameAP-styled UI components. Built as a wrapper around [Naive UI](https://www.naiveui.com/) with sensible defaults and additional custom components.

## Features

- 19 reusable Vue 3 components
- Wrapper components for Naive UI with GameAP defaults
- Flexible icon system with 150+ predefined icons
- Custom menu system with keyboard navigation
- Tailwind CSS integration
- Full TypeScript-friendly props
- Accessibility support (ARIA attributes, keyboard navigation)

## Installation

```bash
npm install @gameap/ui
```

## Setup

Register the plugin in your Vue application:

```javascript
import { createApp } from 'vue'
import gameapUI from '@gameap/ui'
import '@gameap/ui/style.css'

const app = createApp(App)
app.use(gameapUI)
app.mount('#app')
```

Or import components individually:

```javascript
import { GCard, GModal, GIcon } from '@gameap/ui'
```

## Components

### GCard

Card container component wrapping Naive UI's NCard.

**Props:**

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| title | string | `''` | Card title |
| size | string | `'small'` | Card size |
| bordered | boolean | `true` | Show border |
| segmented | boolean \| object | `{ content: true, footer: 'soft' }` | Content segmentation |
| headerClass | string | `'g-card-header'` | Header CSS class |

Supports all [Naive UI NCard props](https://www.naiveui.com/en-US/os-theme/components/card).

**Example:**

```vue
<template>
  <GCard title="Server Status">
    <p>Server is running normally.</p>

    <template #footer>
      <button>Restart Server</button>
    </template>
  </GCard>
</template>
```

---

### GDataTable

Data table component wrapping Naive UI's NDataTable.

**Props:**

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| bordered | boolean | `false` | Show border |
| singleLine | boolean | `true` | Single line rows |
| columns | array | `[]` | Column definitions |
| data | array | `[]` | Table data |
| loading | boolean | `false` | Loading state |
| pagination | object \| boolean | `false` | Pagination config |
| remote | boolean | `false` | Remote data mode |

Supports all [Naive UI NDataTable props](https://www.naiveui.com/en-US/os-theme/components/data-table).

**Example:**

```vue
<script setup>
const columns = [
  { title: 'Name', key: 'name' },
  { title: 'Status', key: 'status' },
  { title: 'Players', key: 'players' }
]

const data = [
  { name: 'Server 1', status: 'Online', players: 12 },
  { name: 'Server 2', status: 'Offline', players: 0 }
]
</script>

<template>
  <GDataTable :columns="columns" :data="data" />
</template>
```

**With Pagination:**

```vue
<script setup>
import { ref } from 'vue'

const pagination = ref({
  page: 1,
  pageSize: 10,
  itemCount: 100,
  showSizePicker: true,
  pageSizes: [10, 20, 50]
})

function handlePageChange(page) {
  pagination.value.page = page
  fetchData()
}
</script>

<template>
  <GDataTable
    :columns="columns"
    :data="data"
    :pagination="pagination"
    :remote="true"
    @update:page="handlePageChange"
  />
</template>
```

---

### GTable

Simple table component wrapping Naive UI's NTable.

**Props:**

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| bordered | boolean | `false` | Show border |
| singleLine | boolean | `true` | Single line rows |
| size | string | - | Table size |

**Example:**

```vue
<template>
  <GTable>
    <thead>
      <tr>
        <th>Name</th>
        <th>Value</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td>CPU Usage</td>
        <td>45%</td>
      </tr>
      <tr>
        <td>Memory</td>
        <td>2.4 GB</td>
      </tr>
    </tbody>
  </GTable>
</template>
```

---

### GModal

Modal dialog component wrapping Naive UI's NModal.

**Props:**

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| show | boolean | `false` | Modal visibility |
| preset | string | `'card'` | Modal preset |
| bordered | boolean | `false` | Show border |
| title | string | `''` | Modal title |
| segmented | object | `{ content: 'soft', footer: 'soft' }` | Content segmentation |

**Emits:**

| Event | Payload | Description |
|-------|---------|-------------|
| update:show | boolean | Emitted when modal should close |

Supports all [Naive UI NModal props](https://www.naiveui.com/en-US/os-theme/components/modal).

**Example:**

```vue
<script setup>
import { ref } from 'vue'

const showModal = ref(false)
</script>

<template>
  <button @click="showModal = true">Open Modal</button>

  <GModal
    v-model:show="showModal"
    title="Confirm Action"
  >
    <p>Are you sure you want to proceed?</p>

    <template #footer>
      <button @click="showModal = false">Cancel</button>
      <button @click="confirmAction">Confirm</button>
    </template>
  </GModal>
</template>
```

---

### GInput

Input field component wrapping Naive UI's NInput.

**Props:**

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| value | string \| array | - | Input value |
| type | string | `'text'` | Input type |
| placeholder | string | `''` | Placeholder text |
| disabled | boolean | `false` | Disabled state |
| readonly | boolean | `false` | Read-only state |
| clearable | boolean | `false` | Show clear button |
| size | string | - | Input size |

**Emits:**

| Event | Payload | Description |
|-------|---------|-------------|
| update:value | string | Emitted when value changes |

Supports all [Naive UI NInput props](https://www.naiveui.com/en-US/os-theme/components/input).

**Example:**

```vue
<script setup>
import { ref } from 'vue'

const serverName = ref('')
</script>

<template>
  <GInput
    v-model:value="serverName"
    placeholder="Enter server name"
    clearable
  />
</template>
```

**Password Input:**

```vue
<template>
  <GInput
    v-model:value="password"
    type="password"
    placeholder="Enter password"
    show-password-on="click"
  />
</template>
```

---

### GSwitch

Toggle switch component wrapping Naive UI's NSwitch.

**Props:**

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| value | boolean | `false` | Switch state |
| disabled | boolean | `false` | Disabled state |
| size | string | - | Switch size |

**Emits:**

| Event | Payload | Description |
|-------|---------|-------------|
| update:value | boolean | Emitted when state changes |

**Example:**

```vue
<script setup>
import { ref } from 'vue'

const autoStart = ref(true)
</script>

<template>
  <label>
    Auto-start server
    <GSwitch v-model:value="autoStart" />
  </label>
</template>
```

---

### GEmpty

Empty state component wrapping Naive UI's NEmpty.

**Props:**

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| description | string | - | Description text |
| size | string | - | Component size |

**Example:**

```vue
<template>
  <GEmpty description="No servers found">
    <template #extra>
      <button>Add Server</button>
    </template>
  </GEmpty>
</template>
```

---

### GDivider

Visual divider component wrapping Naive UI's NDivider.

**Example:**

```vue
<template>
  <div>Section 1</div>
  <GDivider />
  <div>Section 2</div>

  <!-- With label -->
  <GDivider>Or</GDivider>
</template>
```

---

### GIcon

Flexible icon component supporting Font Awesome classes, inline SVG data and Vue components.

**Props:**

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| name | string | *required* | Icon name from registry |
| size | string | `'md'` | Size: `'sm'`, `'md'`, `'lg'`, `'xl'` |
| class | string | `''` | Additional CSS classes |

**Size Mappings:**

| Size | Font Awesome | Component |
|------|--------------|-----------|
| sm | fa-sm | 0.875em |
| md | (default) | 1em |
| lg | fa-lg | 1.25em |
| xl | fa-2x | 2em |

**Example:**

```vue
<template>
  <!-- Basic usage -->
  <GIcon name="server" />

  <!-- With size -->
  <GIcon name="warning" size="lg" />

  <!-- With custom class -->
  <GIcon name="delete" class="text-red-500" />
</template>
```

---

### GGameIcon

Game-specific icon component that displays icons based on game codes.

**Props:**

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| game | string | `'minecraft'` | Game code identifier |

**Supported Games:**

The component includes built-in icon mappings for popular games:
- Counter-Strike series: `cs2`, `csgo`, `css`, `cstrike`, `cs15`, `czero`
- Valve games: `halflife`, `tf2`, `l4d`, `l4d2`, `dod`, `dods`, `garrysmod`
- Other popular games: `minecraft`, `rust`, `ark`, `arma2`, `arma3`, `7d2d`, `dst`, `enshrouded`, `etlegacy`, `factorio`, `fivem`, `gta`, `hurtworld`, `palworld`, `pz`, `quake`, `quake2`, `quake3`, `samp`, `teamspeak`

Game logos resolve to `game-*` entries in the icon registry, rendered as inline SVG.

For unknown game codes, the component automatically assigns a consistent fallback icon from a set of common gaming icons.

**Example:**

```vue
<template>
  <!-- Basic usage -->
  <GGameIcon game="minecraft" />

  <!-- In a table -->
  <GDataTable :columns="columns" :data="servers">
    <template #game="{ row }">
      <GGameIcon :game="row.gameCode" class="mr-2" />
      {{ row.gameName }}
    </template>
  </GDataTable>
</template>
```

**With render function:**

```vue
<script setup>
import { h } from 'vue'
import { GGameIcon } from '@gameap/ui'

const renderGameLabel = (option) => {
  return [
    h(GGameIcon, { game: option.value, class: 'mr-2' }),
    option.label
  ]
}
</script>
```

---

### GBreadcrumbs

Breadcrumb navigation component with support for links, router links, and icons.

**Props:**

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| items | array | *required* | Breadcrumb items |

**Item Structure:**

```typescript
interface BreadcrumbItem {
  text: string           // Display text
  link?: string          // External URL
  route?: string         // Vue Router route
  icon?: string          // Icon name from the registry, or raw CSS classes
  render?: () => VNode   // Custom render function
}
```

**Example:**

```vue
<script setup>
const breadcrumbs = [
  { text: 'Home', route: '/', icon: 'gameap' },
  { text: 'Servers', route: '/servers' },
  { text: 'Server 1' }
]
</script>

<template>
  <GBreadcrumbs :items="breadcrumbs" />
</template>
```

**With Custom Render:**

```vue
<script setup>
import { h } from 'vue'

const breadcrumbs = [
  { text: 'Home', route: '/' },
  {
    text: 'Status',
    render: () => h('span', { class: 'text-green-500' }, 'Online')
  }
]
</script>

<template>
  <GBreadcrumbs :items="breadcrumbs" />
</template>
```

---

### GStatusBadge

Status indicator badge with predefined color schemes.

**Props:**

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| status | string | *required* | Status type |
| text | string | - | Override default text |

**Available Statuses:**

| Status | Color | Default Text |
|--------|-------|--------------|
| waiting | Stone (light) | waiting |
| working | Blue | working |
| error | Red | error |
| success | Green | success |
| canceled | Stone | canceled |

**Example:**

```vue
<template>
  <GStatusBadge status="success" />

  <!-- With custom text -->
  <GStatusBadge status="working" text="Installing..." />

  <!-- In a table -->
  <GDataTable :columns="columns" :data="data">
    <template #status="{ row }">
      <GStatusBadge :status="row.status" />
    </template>
  </GDataTable>
</template>
```

---

### GDeletableList

List component with delete buttons for each item.

**Props:**

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| items | array | *required* | List items |
| clickCallback | function | - | Called on item click: `(gameCode, id) => void` |
| deleteCallback | function | - | Called on delete click: `(id) => void` |

**Item Structure:**

```typescript
interface ListItem {
  id: any          // Unique identifier
  name: string     // Display name
  gameCode: string // Game code identifier
}
```

**Example:**

```vue
<script setup>
const games = [
  { id: 1, name: 'Counter-Strike 2', gameCode: 'cs2' },
  { id: 2, name: 'Minecraft', gameCode: 'minecraft' },
  { id: 3, name: 'Rust', gameCode: 'rust' }
]

function handleClick(gameCode, id) {
  console.log(`Clicked ${gameCode} with id ${id}`)
}

function handleDelete(id) {
  console.log(`Delete item ${id}`)
}
</script>

<template>
  <GDeletableList
    :items="games"
    :click-callback="handleClick"
    :delete-callback="handleDelete"
  />
</template>
```

---

### Menu System

A set of components for building accessible dropdown menus with keyboard navigation.

#### GMenu

Container component that provides menu context to children.

**Props:**

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| as | string | `'div'` | HTML element tag |

#### GMenuButton

Button to toggle menu visibility.

#### GMenuItems

Container for menu items with visibility control.

**Props:**

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| unmount | boolean | `true` | Unmount when closed (false uses v-show) |

#### GMenuItem

Individual menu item with slot props.

**Slot Props:**

| Prop | Type | Description |
|------|------|-------------|
| active | boolean | Item is currently focused |
| close | function | Close the menu |

**Features:**
- Click-outside detection
- ESC key closes menu
- Enter/Space/ArrowDown opens menu
- Hover state tracking
- ARIA attributes for accessibility

**Example:**

```vue
<template>
  <GMenu>
    <GMenuButton class="btn btn-primary">
      Actions
      <GIcon name="chevron-down" size="sm" />
    </GMenuButton>

    <GMenuItems class="dropdown-menu">
      <GMenuItem v-slot="{ active, close }">
        <button
          :class="{ 'bg-blue-100': active }"
          @click="startServer(); close()"
        >
          <GIcon name="play" /> Start
        </button>
      </GMenuItem>

      <GMenuItem v-slot="{ active, close }">
        <button
          :class="{ 'bg-blue-100': active }"
          @click="stopServer(); close()"
        >
          <GIcon name="stop" /> Stop
        </button>
      </GMenuItem>

      <GMenuItem v-slot="{ active, close }">
        <button
          :class="{ 'bg-red-100': active }"
          @click="deleteServer(); close()"
        >
          <GIcon name="delete" /> Delete
        </button>
      </GMenuItem>
    </GMenuItems>
  </GMenu>
</template>
```

---

### Loading

Animated loading spinner component.

**Example:**

```vue
<template>
  <Loading v-if="isLoading" />
  <div v-else>
    Content loaded
  </div>
</template>
```

---

### Progressbar

Linear progress bar with percentage display.

**Props:**

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| progress | number | `0` | Progress percentage (0-100) |

**Example:**

```vue
<script setup>
import { ref } from 'vue'

const downloadProgress = ref(45)
</script>

<template>
  <Progressbar :progress="downloadProgress" />
</template>
```

---

## Icon System

The package includes a flexible icon registry system.

### Icon Registry API

```javascript
import {
  registerIcons,
  getIcon,
  hasIcon,
  iconRegistry,
  defaultIconMap,
  defineSvgIcon
} from '@gameap/ui'

// Register custom icons
registerIcons({
  'my-icon': 'fa-solid fa-star',
  'custom-component': MyIconComponent,
  'custom-svg': defineSvgIcon({
    viewBox: '0 0 24 24',
    paths: ['M12 2 2 22h20L12 2z']
  })
})

// Check if icon exists
if (hasIcon('server')) {
  const icon = getIcon('server')
}

// Access all icons
console.log(iconRegistry)
```

### Default Icon Categories

The package includes 150+ predefined icon mappings:

**Action Icons:**
`delete`, `edit`, `save`, `add`, `close`, `copy`, `paste`, `cut`, `refresh`, `download`, `upload`, `search`, `view`, `clear`, `eraser`, `ban`, `move`

**Navigation Icons:**
`chevron-left`, `chevron-right`, `chevron-up`, `chevron-down`, `arrow-up`, `arrow-down`, `sort-asc`, `sort-desc`, `external-link`

**Status Icons:**
`check`, `warning`, `error`, `info`, `question`, `online`, `offline`, `certificate`, `heart-pulse`, `power-off`

**UI/State Icons:**
`loading`, `spinner`, `play`, `pause`, `stop`, `restart`

**Server/Infrastructure Icons:**
`server`, `node`, `hard-drive`, `terminal`, `console`, `gamepad`, `tasks`, `plug`, `memory`, `cpu`

**User/Auth Icons:**
`user`, `users`, `user-edit`, `login`, `logout`, `key`, `lock`, `address-card`, `profile`

**File Icons:**
`file`, `file-code`, `file-text`, `file-pdf`, `file-word`, `file-excel`, `file-archive`, `file-stack`, `folder`, `folder-open`, `clipboard`, `ftp`

**Brand Icons:**
`linux`, `windows`, `apple`, `telegram`, `discord`, `vk`, `reddit`, `patreon`, `teamspeak`

**Game Icons:**
`dice`, `dice-one` through `dice-six`, `cat`, `mods`, `kick`

**Game Logos** (inline SVG, used by `GGameIcon`):
`game-ark-survival-evolved`, `game-arma-2`, `game-arma-3`, `game-black-mesa`, `game-counter-strike`,
`game-counter-strike-1`, `game-counter-strike-source`, `game-day-of-defeat`, `game-dont-starve`,
`game-enshrouded`, `game-etlegacy`, `game-factorio`, `game-fivem`, `game-garrys-mod`,
`game-grand-theft-auto`, `game-half-life`, `game-hurtworld`, `game-left-4-dead`, `game-minecraft`,
`game-minecraft-creeper`, `game-palworld`, `game-quake`, `game-quake-2`, `game-quake-3`,
`game-rockstar`, `game-rust`, `game-team-fortress-2`, `game-zomboid`

**Theme Icons:**
`sun`, `moon`

### Adding a Game Logo

The SVG files in `icons/assets/` are the single source of truth — they are loaded
directly, with no generation step and nothing to keep in sync. To add a logo:

1. Drop the SVG into `icons/assets/<name>.svg`. It must have a `viewBox` and must not
   set `fill`, so that the icon inherits `currentColor`. At most one wrapping
   `<g transform="...">` is supported; it is carried over to the icon.
2. Register it in `icons/iconMap.js` as `"game-<name>": svgAsset('<name>')` and map the
   game codes to it in `components/GGameIcon.vue`.

`svgAsset()` throws on a name with no matching file, so typos surface at startup rather
than as a missing icon.

Logos that came from the old `gicon` icon font carry a `translate(0 1024) scale(1 -1)`
group, because SVG fonts store glyphs with the y axis pointing up.

Loading uses Vite's `import.meta.glob(..., { query: '?raw' })`, so this package requires
a Vite-based build.

### Custom Icon Registration

Register custom icons at app initialization:

```javascript
import { registerIcons } from '@gameap/ui'
import CustomIcon from './components/CustomIcon.vue'

// Register Font Awesome classes
registerIcons({
  'rocket': 'fa-solid fa-rocket',
  'database': 'fa-solid fa-database'
})

// Register Vue components
registerIcons({
  'custom-logo': CustomIcon
})
```

---

## Theming

The package ships the panel's design tokens in `theme.css` — a set of CSS custom
properties with the `--gameap-` prefix. The panel serves this file verbatim at
`/theme.css` (loaded before all other styles), and every color in the panel's
compiled CSS resolves through these variables, so overriding a variable
re-themes the whole panel at runtime — naive-ui components included.

### Two tiers

1. **Palette** — `--gameap-<family>-<shade>` raw color ramps for the families
   `stone`, `red`, `orange`, `lime`, `sky` (shades `50`–`950`, Tailwind v3
   default values). Palette variables do **not** change between light and dark
   mode.
2. **Semantic** — role tokens. Light values are declared on `:root`, dark
   overrides on `html.dark`.

### Semantic variables

| Variable | Light | Dark | Purpose |
|----------|-------|------|---------|
| `--gameap-surface` | `#ffffff` | `stone-800` | page background |
| `--gameap-surface-raised` | `#ffffff` | `stone-800` | cards (naive `cardColor`) |
| `--gameap-surface-overlay` | `#ffffff` | `stone-800` | modals (naive `modalColor`) |
| `--gameap-surface-hover` | `stone-100` | `#262322` | hovered/selected rows |
| `--gameap-scrim` | `stone-900` 10% | `stone-100` 10% | drag & drop veil |
| `--gameap-border` | `stone-200` | `stone-700` | default border (bare `border` utility) |
| `--gameap-border-strong` | `stone-300` | `stone-600` | emphasized border |
| `--gameap-text` | `stone-900` | `#ffffff` | main text |
| `--gameap-text-secondary` | `stone-600` | `stone-300` | secondary text |
| `--gameap-text-muted` | `stone-500` | `stone-400` | muted text |
| `--gameap-text-faint` | `stone-400` | `stone-500` | faint text |
| `--gameap-primary`, `-hover` | `lime-500` / `lime-600` | same | primary accent |
| `--gameap-primary-soft`, `-soft-text` | `lime-50` / `lime-700` | `lime-950` 40% / `lime-300` | soft badges |
| `--gameap-success*` (4) | mirrors primary | mirrors primary | success intent |
| `--gameap-danger*` (4) | `red-500/600/50/700` | soft: `red-950` 40% / `red-300` | danger intent |
| `--gameap-warning*` (4) | `orange-400/500/50/700` | soft: `orange-950` 40% / `orange-300` | warning intent |
| `--gameap-info*` (4) | `sky-500/600/50/700` | soft: `sky-950` 40% / `sky-300` | info intent |
| `--gameap-chrome`, `-item`, `-hover` | `stone-900/800/700` | same | navbar + sidebar (never flips) |
| `--gameap-table-header` | `stone-100` | `stone-700` | naive `tableHeaderColor` |
| `--gameap-tab-accent` | `stone-900` | `#737373` | naive Tabs active text/bar |
| `--gameap-ring-subtle` | `stone-500` 10% | same | `.badge-light` ring |
| `--gameap-selection-outline`, `-weak` | `stone-500` 70% / 30% | same | file manager selection |
| `--gameap-terminal-bg`, `-text` | `stone-800` / `stone-100` | `stone-900` / `stone-100` | console terminal |
| `--gameap-chart-1`…`-10` | fixed hexes | same | chart series palette |

### Semantic utility classes

The panel's Tailwind build guarantees these classes exist (safelisted), so
plugin templates can use them; they need no `dark:` twin — the variable flips:

`bg-surface`, `bg-surface-raised`, `bg-surface-overlay`, `bg-surface-hover`,
`bg-scrim`, `bg-terminal`, `bg-chrome`, `bg-chrome-item`,
`bg-{primary,success,danger,warning,info}` (+`-hover`, `-soft`),
`text-{body,secondary,muted,faint}`,
`text-{primary,success,danger,warning,info}` (+`-soft-text`),
`border-strong`, `border-danger`, `border-warning`.
The bare `border` utility resolves to `--gameap-border`.

### Overriding

```css
:root {
    --gameap-primary: #e11d48;
    --gameap-primary-hover: #be123c;
}

html.dark {
    --gameap-surface: #1e1b4b;
}
```

Dark values **must** be overridden under `html.dark` — a plain `:root`
declaration loses to the `html.dark` rules in `theme.css` on specificity.

### Stability

The variables are a public API covered by this package's semver: additions are
a minor release, renames or removals are a major release.

External consumers import the tokens with:

```js
import '@gameap/ui/theme.css'
```

Translucent tokens use `color-mix()`, which requires Chrome 111+/Firefox
113+/Safari 16.2+.

---

## CSS Utilities

The package provides Tailwind CSS utility classes in `style.css`. All colors
resolve through the `--gameap-*` theme variables and can be re-themed (see
[Theming](#theming)).

### Badge Classes

```html
<span class="badge-red">Error</span>
<span class="badge-green">Success</span>
<span class="badge-orange">Warning</span>
<span class="badge-blue">Info</span>
<span class="badge-stone">Neutral</span>
<span class="badge-light">Light</span>
```

All badge classes include dark mode support.

### Progress Classes

```html
<div class="progress">
  <div class="progress-bar progress-bar-info" style="width: 50%">
    50%
  </div>
</div>
```

---

## Dependencies

### Peer Dependencies

| Package | Version | Required |
|---------|---------|----------|
| vue | ^3.5.0 | Yes |
| vue-router | ^4.0.0 | Optional |

### Runtime Dependencies

- **naive-ui** - Base UI component library

### Build Requirements

- **Vite** - `icons/svgAssets.js` loads `icons/assets/*.svg` via `import.meta.glob`

### Styling Requirements

- **Tailwind CSS** - Must be configured in the consuming application
- **Font Awesome** - Required for default icon mappings

---

## Browser Support

Supports all modern browsers that support Vue 3:
- Chrome (latest)
- Firefox (latest)
- Safari (latest)
- Edge (latest)

---

## License

Part of the GameAP project.
