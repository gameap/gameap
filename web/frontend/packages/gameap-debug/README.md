# GameAP Plugin Debug Harness

A full-fledged development environment for debugging GameAP frontend plugins. This harness runs the **real GameAP frontend** with **Mock Service Worker (MSW)** providing realistic API responses.

## Features

- **Real Frontend**: Uses the actual GameAP frontend application, not a simplified mock
- **MSW Mock API**: All API endpoints are mocked with realistic data using Mock Service Worker
- **Debug Panel**: Floating panel to switch user types, adjust network delays, and change locales
- **Plugin Hot Reload**: Changes to your plugin source are reflected after rebuild
- **File Manager Testing**: Full file manager with mock file system
- **Server Management**: Mock servers with console, RCON, and file access
- **Authentication Testing**: Switch between admin, regular user, and guest modes

## Quick Start

### Prerequisites

1. Install dependencies from the frontend root:
```bash
cd /path/to/gameap-api/web/frontend
npm install
```

2. Initialize MSW service worker:
```bash
cd packages/gameap-debug
npm run msw:init
```

### Running the Debug Harness

```bash
# From the gameap-debug directory
cd packages/gameap-debug

# Run with default plugin (hex-editor-plugin)
npm run dev

# Or run with a custom plugin path
PLUGINS_PATH=/path/to/my-plugin/frontend/dist npm run dev
```

The debug harness will start at `http://localhost:5174`

### Testing Your Plugin

1. **Build your plugin first:**
```bash
cd /path/to/my-plugin/frontend
npm run build
```

2. **Run the debug harness with your plugin:**
```bash
PLUGINS_PATH=/path/to/my-plugin/frontend/dist npm run dev
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PLUGINS_PATH` | Path to your plugin's dist directory (built bundle). The former `PLUGIN_PATH` still works and warns. | hex-editor-plugin dist |
| `LOCALE` | Default locale (en/ru) | `en` |

## Debug Panel

The debug panel appears in the bottom-right corner and allows you to:

- **User Type**: Switch between Admin, Regular User, and Guest (unauthenticated)
- **Network Delay**: Adjust API response delay (useful for testing loading states)
- **Locale**: Switch between English and Russian

Changes to user type require a page reload to take effect.

## Mock Data

### Servers

| ID | Name | Game | Port | Status |
|----|------|------|------|--------|
| 1 | Minecraft Survival | minecraft | 25565 | Running |
| 2 | CS2 Competitive | cs2 | 27015 | Stopped |

### Users

| Type | Login | Access |
|------|-------|--------|
| Admin | admin | Full administrative access |
| User | player1 | Regular user permissions |
| Guest | - | Not authenticated |

### Mock Files

| Path | Type | Description |
|------|------|-------------|
| `/server.properties` | text | Minecraft server config |
| `/cstrike/server.cfg` | text | CS server config |
| `/config/config.json` | text | JSON config file |
| `/readme.txt` | text | Plain text file |
| `/data/sample.dat` | binary | 256 bytes (0x00-0xFF) |
| `/data/complex.bin` | binary | 1KB with header pattern |

## API Mocking

All GameAP API endpoints are mocked using MSW:

### Available Endpoints

- **Auth**: `/api/profile`, `/api/user/servers_abilities`
- **Servers**: `/api/servers`, `/api/servers/:id`, `/api/servers/:id/abilities`
- **Server Control**: `/api/servers/:id/start|stop|restart`
- **Console**: `/api/servers/:id/console`
- **RCON**: `/api/servers/:id/rcon/*`
- **File Manager**: `/api/servers/:id/filemanager/*`
- **Games**: `/api/games`, `/api/game_mods`
- **Nodes**: `/api/nodes`
- **Users**: `/api/users`
- **Tasks**: `/api/gdaemon_tasks`
- **Plugins**: `/plugins.js`, `/plugins.css`

### Customizing Mock Responses

You can customize mock behavior by modifying `src/mocks/handlers.ts`:

```typescript
import { debugState } from './mocks/handlers'

// Change network delay
debugState.networkDelay = 500 // 500ms delay

// Change user type
debugState.userType = 'guest'

// Change locale
debugState.locale = 'ru'
```

## Plugin Development Workflow

1. **Create your plugin** using the Plugin SDK
2. **Build your plugin**: `npm run build`
3. **Run the debug harness**: `PLUGINS_PATH=./dist npm run dev`
4. **Make changes** to your plugin
5. **Rebuild**: `npm run build`
6. **Refresh** the debug harness page

### Hot Reload Setup (Optional)

Add a watch script to your plugin's package.json:

```json
{
  "scripts": {
    "dev:watch": "vite build --watch",
    "debug": "PLUGINS_PATH=./dist vite --config ../../gameap-api/web/frontend/packages/gameap-debug/vite.config.ts"
  }
}
```

Run both in separate terminals:
```bash
# Terminal 1: Watch plugin source
npm run dev:watch

# Terminal 2: Run debug harness
npm run debug
```

## Architecture

```
packages/gameap-debug/
├── src/
│   ├── main.ts           # Entry point - initializes MSW, loads real app
│   └── mocks/
│       ├── browser.ts    # MSW browser setup
│       ├── handlers.ts   # API mock handlers
│       ├── files.ts      # Mock file system data
│       ├── servers.ts    # Mock server data
│       └── users.ts      # Mock user data
├── index.html            # Debug harness HTML
├── vite.config.ts        # Vite config with aliases
└── package.json
```

## Troubleshooting

### "Failed to load plugin bundle"

- Ensure your plugin is built (`npm run build`)
- Check that `PLUGINS_PATH` points to the `dist` directory
- Verify the plugin exports a valid `PluginDefinition`

### MSW not intercepting requests

- Run `npm run msw:init` to create the service worker file
- Check browser console for MSW initialization messages
- Ensure requests go to the same origin (no CORS issues)

### Plugin context hooks not working

The harness provides the same injection keys as the main app:
- `pluginContext` for `usePluginContext()`, `useServer()`, etc.
- `pluginI18n` for `usePluginTrans()`

### Styles not loading

- Ensure your plugin CSS is exported as `style.css` in the dist folder
- Check browser console for CSS loading errors

## Global Debug API

Access debug utilities from the browser console:

```javascript
// Update debug state
window.gameapDebug.updateDebugState({ userType: 'guest' })

// Load new plugin content
window.gameapDebug.loadPlugin(jsContent, cssContent)
```
