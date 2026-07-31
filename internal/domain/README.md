# Domain Models

This directory contains the core domain models for the GameAP API. These models represent the fundamental business entities and their relationships within the game server management system.

## Core Entities

### User (`user.go`)
Represents system users who can manage game servers and access the GameAP platform.

### Server (`server.go`)
Represents a game server instance with its configuration, network settings, resource limits, and lifecycle commands.

### Node (`node.go`)
Represents a dedicated server (physical or virtual machine) that hosts game servers. Contains connection settings for GameAP Daemon and server management scripts.

> **Security — `ScriptSendCommand` quoting.** The user-supplied `{command}`
> placeholder is now shell-escaped (wrapped in single quotes via
> `pkg/shellescape`) before substitution to prevent command injection on the
> node. Configure the template **without** quoting `{command}` yourself:
> use `tmux send-keys -t {uuid_short} {command} ENTER`, not
> `... "{command}" ENTER`. A template that wraps `{command}` in its own quotes
> will now send the literal quote characters to the game console.

## Game Management

### Game (`game.go`)
Represents a base game definition with engine information and installation repositories for Linux/Windows platforms.

### GameMod (`game_mod.go`)
Represents a game modification or variant with RCON commands, game variables, and server control commands.

## Authentication & Authorization

### Auth (`auth.go`)
Personal access token system for API authentication with scoped abilities for server control and admin operations.

### RBAC (`rbac.go`)
Role-Based Access Control system for fine-grained permissions. Includes roles, abilities, permissions, and entity-based access restrictions.

### ClientCertificate (`client_certificate.go`)
SSL/TLS certificates for secure communication with GameAP Daemon.

## Task Management

### DaemonTask (`gdaemon_task.go`)
Low-level tasks executed by GameAP Daemon on nodes. Supports server lifecycle operations, updates, and custom command execution.

### ServerTask (`server_task.go`)
High-level scheduled tasks for game servers. Definition only — execution lives in the daemon. Supports recurring runs, soft-delete, version/ETag for safe live edits, per-task overlap and catch-up policies.

### ServerTaskExecution (`server_task_execution.go`)
Unified audit log of scheduled task runs (success + failure). Daemon-generated `execution_id` (UUID) is the idempotency key. Status covers `running`/`success`/`failed`/`canceled`/`skipped`/`timed_out`. Large stdout/stderr is offloaded to file-transfer storage via `OutputStoragePath`; `OutputInline` carries the truncated tail.

## Settings

### ServerSetting (`server_setting.go`)
Key-value configuration storage for individual game servers with type-flexible values (string, boolean, integer).

## Plugin System

### Plugin (`plugin.go`)
Represents a WebAssembly plugin with metadata, event hooks, and HTTP route registrations for extending GameAP functionality.

### PluginStorageEntry (`plugin_storage.go`)
Persistent key-value storage for plugins, allowing them to store and retrieve data associated with specific entities (servers, users, etc.).

### PluginScheduledTask (`plugin_scheduled_task.go`)
Definition of a periodic task registered by a plugin via the gameap-scheduler host module: interval, error policy (ignore/retry with delay and jitter) and per-run timeout. Run state is not persisted; panel instances coordinate runs through distributed locks.