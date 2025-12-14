# Server Logger Plugin

This plugin logs server lifecycle events and provides statistics via HTTP API and a Vue.js frontend.

## Features

- Subscribes to all server lifecycle events (start, stop, restart, install, update, reinstall, delete)
- Provides HTTP API endpoints for status and statistics
- Includes a Vue.js frontend with dashboard widget and server tab

## Building

### 1. Build Frontend

```bash
cd frontend
npm install
npm run build
```

### 2. Build WASM Plugin

```bash
tinygo build -o server-logger.wasm -target=wasip1 -buildmode=c-shared -scheduler=asyncify .
```

## HTTP API Endpoints

- `GET /status` - Get plugin status (no auth required)
- `GET /stats` - Get plugin statistics (requires auth)
- `GET /servers/{id}` - Get server info by ID (requires auth)

## Frontend Components

- **Dashboard Widget** - Shows event processing statistics
- **Server Tab** - Shows server-specific information from the plugin API
- **Plugin Page** - Main page with status, stats, and about information
