# GameAP Plugin Debug Harness

A standalone development environment for debugging GameAP frontend plugins without running the full GameAP backend.

## Features

- **File Editor Testing**: Test your file editors with sample binary and text files
- **Server Tab Testing**: Test server tab components with mock server data
- **Route Testing**: Navigate and test plugin routes
- **Debug Panel**: Toggle user types (admin/user/guest), servers (Minecraft/CS), and locales (en/ru)
- **Event Logging**: Monitor save, close, and other events emitted by your plugin
- **Hot Module Replacement**: Changes to your plugin source are reflected immediately

## Quick Start

### Default usage (hex-editor-plugin)

```bash
# From the gameap-debug directory
cd /path/to/gameap-api/web/frontend/packages/gameap-debug

# Run the debug harness
npm run dev
```

This will load the built hex-editor-plugin by default.

### Testing your own plugin

```bash
# Point to your plugin's dist directory (after building)
PLUGIN_PATH=/path/to/my-plugin/frontend/dist npm run dev
```

### Add to your plugin's package.json

```json
{
  "scripts": {
    "debug": "npm run build && PLUGIN_PATH=./dist vite --config ../../gameap-api/web/frontend/packages/gameap-debug/vite.config.ts"
  }
}
```

Then run:
```bash
npm run debug
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PLUGIN_PATH` | Path to your plugin's dist directory (built bundle) | hex-editor-plugin dist |
| `LOCALE` | Default locale (en/ru) | `en` |

## Important Notes

1. **Build your plugin first**: The harness loads the built plugin bundle (`plugin.js`), not the source. Run `npm run build` in your plugin before testing.

2. **Plugin CSS**: The harness also imports the plugin's CSS file (e.g., `hex-editor-plugin.css`).

3. **Tailwind v4**: Both the harness and plugins should use Tailwind CSS v4 for compatibility.

## Testing File Editors

The harness includes sample test files:

| File | Type | Description |
|------|------|-------------|
| `server.properties` | text | Minecraft server config |
| `server.cfg` | text | Counter-Strike server config |
| `config.json` | text | JSON configuration file |
| `readme.txt` | text | Simple text file |
| `sample.dat` | binary | 256 bytes (0x00-0xFF pattern) |
| `complex.bin` | binary | 1KB with header pattern |

## Mock Data

### Servers

- **Minecraft**: `game_id: 'minecraft'`, port 25565, running state
- **Counter-Strike**: `game_id: 'cs2'`, port 27015, stopped state

### Users

- **Admin**: Full admin access, authenticated
- **Regular User**: Non-admin, authenticated
- **Guest**: Not authenticated

## Debug Panel Controls

The debug panel (bottom-right corner) allows you to:

- Switch between user types to test permission handling
- Change the active server context
- Toggle between English and Russian locales
- Enable/disable dark mode
- View current plugin info

## Plugin Requirements

Your plugin must export a `PluginDefinition` object. The harness will search for:

1. Default export
2. Named exports: `plugin`, `hexEditorPlugin`, `myPlugin`
3. Any export matching the `PluginDefinition` interface

Example:

```typescript
import type { PluginDefinition } from '@gameap/plugin-sdk';

export const myPlugin: PluginDefinition = {
    id: 'my-plugin',
    name: 'My Plugin',
    version: '1.0.0',
    apiVersion: '1.0',
    fileEditors: [...],
    slots: {...},
    routes: [...],
};
```

## Directory Structure

```
packages/gameap-debug/
├── src/
│   ├── main.ts              # Entry point, plugin loading
│   ├── App.vue              # Main shell
│   ├── stores/              # Pinia stores
│   │   ├── debug.ts         # Debug panel state
│   │   └── plugins.ts       # Plugin registry
│   ├── context/             # Plugin context provider
│   ├── mocks/               # Mock data
│   ├── components/          # UI components
│   └── views/               # Test views
├── index.html
├── vite.config.ts
└── package.json
```

## Troubleshooting

### "No plugin definition found"

Make sure your plugin exports a `PluginDefinition` object with at least `id`, `name`, and `version` fields.

### Plugin components not rendering

Check that:
1. Your plugin's dependencies (Vue, VueRouter, Pinia) use global imports from `window`
2. The plugin SDK alias is correctly configured

### Context hooks not working

The harness provides the same injection keys as the main app:
- `pluginContext` for `usePluginContext()`, `useServer()`, etc.
- `pluginI18n` for `usePluginTrans()`

## Development

```bash
# Install dependencies (from frontend root)
cd /path/to/gameap-api/web/frontend
yarn install

# Run the debug harness directly
cd packages/gameap-debug
npm run dev
```
