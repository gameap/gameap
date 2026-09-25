# GameAP v4 API - OpenAPI 3.1 Documentation

## Overview

This directory contains the OpenAPI 3.1 specification for GameAP v4 API.

## Structure

```
openapi/
├── openapi.yaml          # Main entry point
├── README.md             # This file
├── paths/                # API endpoint definitions
├── schemas/              # Reusable data models
├── parameters/           # Reusable parameter definitions
└── security/             # Authentication scheme definitions
```

### `/paths/`

API endpoint definitions organized by resource:

| File | Description |
|------|-------------|
| `health.yaml` | Health check endpoint |
| `config.yaml` | Public configuration |
| `auth.yaml` | Authentication (login) |
| `user.yaml` | Current user info |
| `profile.yaml` | User profile management |
| `tokens.yaml` | Personal Access Tokens |
| `servers.yaml` | Game server CRUD |
| `servers-control.yaml` | Server start/stop/restart/update |
| `servers-console.yaml` | Server console access |
| `servers-rcon.yaml` | RCON commands |
| `servers-tasks.yaml` | Scheduled tasks |
| `servers-settings.yaml` | Server settings |
| `servers-abilities.yaml` | Server abilities |
| `file-manager.yaml` | File operations |
| `users.yaml` | User management (Admin) |
| `nodes.yaml` | Node management (Admin) |
| `games.yaml` | Game management (Admin) |
| `game-mods.yaml` | Game mod management (Admin) |
| `daemon-tasks.yaml` | Daemon tasks (Admin) |
| `client-certificates.yaml` | Certificates (Admin) |
| `plugin-store.yaml` | Plugin store (Admin) |
| `gdaemon-setup.yaml` | Daemon setup |
| `gdaemon-api-*.yaml` | Daemon API endpoints |

### `/schemas/`

Reusable data models:

- **Domain models**: Server, User, Game, GameMod, Node, etc.
- **Common types**: Enums, pagination, errors
- **Requests**: Input schemas for POST/PUT operations
- **Responses**: Output schemas for specific endpoints

### `/parameters/`

Reusable parameter definitions for path, query and header parameters.

## API Groups

### 1. Main API (`/api/*`)

Primary API for web UI and third-party integrations.

**Authentication**: Bearer token (JWT/PASETO) or Personal Access Token

**Endpoint groups**:
- Public: health, config
- User: current user, profile, tokens
- Servers: CRUD, control, console, RCON, tasks, settings, file manager
- Admin: users, nodes, games, game mods, daemon tasks, certificates, plugins

### 2. Daemon Setup (`/gdaemon/*`)

One-time setup endpoints for new node registration.

**Authentication**: Setup token in URL path

### 3. Daemon API (`/gdaemon_api/*`)

Internal API for daemon-to-panel communication.

**Authentication**: X-Auth-Token header

## Authentication

### Bearer Token (JWT/PASETO)

```http
Authorization: Bearer eyJhbGciOiJIUzM4NCIsInR5cCI6IkpXVCJ9...
```

Obtained from `POST /api/auth/login`. Valid for 24 hours (30 days with `remember_me`).

### Personal Access Token

```http
Authorization: Bearer 42|abc123def456...
```

Format: `{token_id}|{token_secret}`. Created via `POST /api/tokens`.

### Daemon Token

```http
X-Auth-Token: daemon-api-token-here
```

Used only for `/gdaemon_api/*` endpoints.

## Usage

### Viewing Documentation

Using Redocly:
```bash
npx @redocly/cli preview-docs openapi.yaml
```

Using Swagger UI:
```bash
npx swagger-ui-watcher openapi.yaml
```

### Validating Specification

```bash
npx @redocly/cli lint openapi.yaml
```

### Generating HTML Documentation

```bash
npx @redocly/cli build-docs openapi.yaml -o api-docs.html
```

## Contributing

When adding new endpoints:

1. Add path definition to appropriate file in `/paths/`
2. Create/update schemas in `/schemas/`
3. Add reusable parameters to `/parameters/` if needed
4. Update `openapi.yaml` path references
5. Validate with lint command

## Version

- **API Version**: v4
- **OpenAPI Version**: 3.1.0
