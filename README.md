# GameAP

[![Coverage Status](https://coveralls.io/repos/github/gameap/gameap/badge.svg?branch=main)](https://coveralls.io/github/gameap/gameap?branch=main)
![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/gameap/gameap/.github%2Fworkflows%2Ftest.yaml)
![Discord](https://img.shields.io/discord/527221172144701440)

GameAP is a free and open-source game server management panel that allows you to easily manage and deploy game servers.
It provides a user-friendly web interface for managing game servers, users, and configurations.

This repository is under active development.

## Requirements

You don't need to pre-install any dependencies. 
GameAP is distributed as a single binary file that includes all necessary dependencies.

You don't need any special hardware to run the application. 
A basic server with at least 1GB of RAM and a modern CPU should be sufficient for small to medium-sized deployments.

You can run the panel on different operating systems and database backends.

### Operating System

GameAP can be installed on the following operating systems:
- Linux (Ubuntu, Debian, CentOS, etc.)
- Windows Server (2016, 2019, 2022), Windows 10, Windows 11
- MacOS

### Database

GameAP supports the following databases:
- PostgreSQL
- MySQL / MariaDB
- SQLite
- Inmemory (for testing purposes only). Not persistent, data will be lost on restart.

## Configuration

GameAP is configured via environment variables. Below are the available configuration options:

### Server Configuration

- `HTTP_HOST` - HTTP server host (default: `0.0.0.0`)
- `HTTP_PORT` - HTTP server port (default: `8000`)

### Database Configuration

- `DATABASE_DRIVER` - Database driver (required, options: `mysql`, `postgres`, `sqlite`, `inmemory`)
- `DATABASE_URL` - Database connection URL (required)
  - MySQL: `username:password@tcp(host:port)/database?parseTime=true`
  - PostgreSQL: `postgres://username:password@host:port/database?sslmode=disable`
  - SQLite: `file:path/to/database.db`
  - Inmemory: For `inmemory`, this can be left empty.

### Security Configuration

- `ENCRYPTION_KEY` - Encryption key for sensitive data
- `AUTH_SECRET` - Secret key for PASETO/JWT token generation (if not set, uses `ENCRYPTION_KEY`)
- `AUTH_SERVICE` - Authentication service type (default: `paseto`)

### RBAC Configuration

- `RBAC_CACHE_TTL` - Role-based access control cache TTL (default: `30s`)

### Cache Configuration

- `CACHE_DRIVER` - Cache driver (default: `memory`)

### File Storage Configuration

- `FILES_DRIVER` - File storage driver (options: `local`, `s3`)

#### Local Storage

Used when `FILES_DRIVER` is set to `local`.

- `FILES_LOCAL_BASE_PATH` - Base path for local file storage

#### S3 Storage

Used when `FILES_DRIVER` is set to `s3`.

- `FILES_S3_ENDPOINT` - S3-compatible endpoint URL
- `FILES_S3_USE_SSL` - Use SSL for S3 connections (default: `true`)
- `FILES_S3_ACCESS_KEY_ID` - S3 access key ID
- `FILES_S3_SECRET_ACCESS_KEY` - S3 secret access key
- `FILES_S3_BUCKET` - S3 bucket name

### Legacy Configuration

- `LEGACY_PATH` - Path to legacy GameAP installation (default: `/var/www/gameap/`)

### Global API Configuration

- `GLOBAL_API_URL` - Global GameAP API URL for game updates (default: `https://api.gameap.com`)

### Example Configuration

```bash
# Server
HTTP_HOST=0.0.0.0
HTTP_PORT=8000

# Database
DATABASE_DRIVER=mysql
DATABASE_URL=gameap:password@tcp(localhost:3306)/gameap?parseTime=true

# Security
ENCRYPTION_KEY=your-secure-encryption-key-here
AUTH_SECRET=your-secure-auth-secret-here
AUTH_SERVICE=paseto

# Cache
CACHE_DRIVER=memory

# File Storage
FILES_DRIVER=local
FILES_LOCAL_BASE_PATH=/var/gameap/files

# Legacy
LEGACY_PATH=/var/www/gameap/

# Global API
GLOBAL_API_URL=https://api.gameap.com
```