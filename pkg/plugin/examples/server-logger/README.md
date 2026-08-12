# Server Logger Plugin

This plugin logs server lifecycle events and provides statistics via HTTP API and a Vue.js frontend.

## Features

- Subscribes to all server lifecycle events (start, stop, restart, install, update, reinstall, delete)
- Provides HTTP API endpoints for status and statistics
- Includes a Vue.js frontend with dashboard widget and server tab
- Contributes static assets via `GetAssets` (see `assets/`): a Spanish translation served at
  `/lang/es.json` and a namespaced frontend file served at `/plugins/server-logger/meta.json`
- Registers a periodic `stats-report` scheduled task (every 5 minutes, retry policy with jitter) via the gameap-scheduler module (`scheduled.go`)
- Registers an `ArchiveEventsHandler` that logs progress/completion of archive operations started through the gameap-nodefs module (`archive.go`)

## Building

### 1. Build Frontend

```bash
cd frontend
npm install
npm run build
```

### 2. Build WASM Plugin

**Using TinyGo** (smaller binary, ~1MB):
```bash
tinygo build -o server-logger.wasm -target=wasip1 -buildmode=c-shared -scheduler=asyncify .
```

**Using standard Go compiler** (larger binary, ~12MB):
```bash
GOOS=wasip1 GOARCH=wasm go build -o server-logger.wasm -buildmode=c-shared .
```

Use the standard Go compiler if TinyGo doesn't support your Go version.

## Refreshing the test fixture

The `pkg/plugin` test suite loads this plugin from an embedded gzipped copy under
`pkg/plugin/testdata/server-logger.wasm.gz` so tests do not depend on a freshly
built artifact at runtime. After changing the example, regenerate the fixture:

```bash
cd pkg/plugin/examples/server-logger
npm --prefix frontend install && npm --prefix frontend run build
GOOS=wasip1 GOARCH=wasm go build -o server-logger.wasm -buildmode=c-shared .
gzip -9 -c server-logger.wasm > ../../testdata/server-logger.wasm.gz
```

Commit the updated `pkg/plugin/testdata/server-logger.wasm.gz` together with
your code change.

## HTTP API Endpoints

- `GET /status` - Get plugin status (no auth required)
- `GET /stats` - Get plugin statistics (requires auth)
- `GET /servers/{id}` - Get server info by ID (requires auth)

## Frontend Components

- **Dashboard Widget** - Shows event processing statistics
- **Server Tab** - Shows server-specific information from the plugin API
- **Plugin Page** - Main page with status, stats, and about information
