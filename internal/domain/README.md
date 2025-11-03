# Domain Models

This directory contains the core domain models for the GameAP API. These models represent the fundamental business entities and their relationships within the game server management system.

## Core Entities

### User (`user.go`)
Represents system users who can manage game servers and access the GameAP platform.

### Server (`server.go`)
Represents a game server instance with its configuration, network settings, resource limits, and lifecycle commands.

### Node (`node.go`)
Represents a dedicated server (physical or virtual machine) that hosts game servers. Contains connection settings for GameAP Daemon and server management scripts.

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
High-level scheduled tasks for game servers with support for recurring execution and failure logging.

## Settings

### ServerSetting (`server_setting.go`)
Key-value configuration storage for individual game servers with type-flexible values (string, boolean, integer).