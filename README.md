[![GameAP Logo](web/frontend/public/gameap_full.svg)](https://gameap.com)

# GameAP

[![Coverage Status](https://coveralls.io/repos/github/gameap/gameap/badge.svg?branch=main)](https://coveralls.io/github/gameap/gameap?branch=main)
![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/gameap/gameap/.github%2Fworkflows%2Ftest.yaml)
![Discord](https://img.shields.io/discord/527221172144701440)

GameAP is a free and open-source game server management panel that allows you to easily manage and deploy game servers.
It provides a user-friendly web interface for managing game servers, users, and configurations.

Demo: https://demo.gameap.com

## Requirements

You don't need to pre-install any dependencies. 
GameAP is distributed as a single binary file that includes all necessary dependencies.

You don't need any special hardware to run the application. 
A basic server with at least 1GB of RAM and a modern CPU should be sufficient for small to medium-sized deployments.

You can run the panel on different operating systems and database backends.

### Operating System

GameAP can be installed on the following operating systems:
- Linux (Ubuntu, Debian, CentOS, etc.)
- Windows Server (2016, 2019, 2022, 2025), Windows 10, Windows 11
- MacOS


### Architecture

GameAP follows a three-tier architecture consisting of the web panel, daemon agents, and game servers.

Administrators interact with GameAP through a browser via HTTP/HTTPS.
The panel provides a web UI, a REST API, and a WebSocket endpoint for real-time events (task output, console streams, server metrics), and stores its data in a database, which can be either PostgreSQL, MySQL, or SQLite, depending on the deployment.

![](docs/gameap_architecture.svg)

Each GameAP instance talks to one or more nodes over a gRPC bidirectional stream (secured with TLS / mTLS). The daemon opens a single long-lived connection to the panel, used for both control commands (task dispatch, console input, file operations) and upstream events (heartbeats, task status, server status, metrics).
On every node, a lightweight agent called GameAP Daemon runs alongside the game servers.
The daemon is responsible for controlling game server processes (starting, stopping, monitoring, and configuring them).
It supports Windows, Linux, and macOS and offers a wide range of configuration options.

### Database

GameAP supports the following databases:
- PostgreSQL
- MySQL / MariaDB
- SQLite
- Inmemory (for testing purposes only). Not persistent, data will be lost on restart.

## Quick Start with Docker

The fastest way to get started with GameAP is using Docker:

```bash
# Using Docker Compose (recommended)
docker-compose up -d

# Or pull and run the pre-built image
docker pull gameap/gameap:latest
docker run -d -p 8025:8025 \
  -e DATABASE_DRIVER=sqlite \
  -e DATABASE_URL=file:/db.sqlite?_busy_timeout=5000&_journal_mode=WAL&cache=shared \
  -e ENCRYPTION_KEY=your-secret-key \
  -e AUTH_SECRET=your-auth-secret \
  gameap/gameap:latest
```

Access GameAP at http://localhost:8025

For detailed Docker deployment instructions, see [DOCKER.md](DOCKER.md).

## Configuration

GameAP is configured via environment variables. Below are the available configuration options:

### Server Configuration

- `HTTP_HOST` - HTTP server host (default: `0.0.0.0`)
- `HTTP_PORT` - HTTP server port (default: `8025`)
- `HTTPS_PORT` - HTTPS server port (default: `443`)

### TLS Configuration

GameAP can terminate TLS itself or, if you prefer, sit behind a reverse proxy.
For self-terminated TLS the panel accepts three certificate sources, evaluated
in this order:

1. **ACME / Let's Encrypt** — `ACME_ENABLED=true` plus the `ACME_*` variables
   below. The panel obtains and renews the certificate automatically.
2. **Static cert files** — `TLS_CERT_FILE` + `TLS_KEY_FILE`.
3. **Inline cert content** — `TLS_CERT` + `TLS_KEY` (raw PEM or base64-encoded).

If none of the three is configured, the HTTPS listener does not start and the
panel only serves plain HTTP on `HTTP_PORT`.

- `TLS_CERT_FILE` - Path to TLS certificate file
- `TLS_KEY_FILE` - Path to TLS private key file
- `TLS_CERT` - TLS certificate content (PEM or base64 encoded)
- `TLS_KEY` - TLS private key content (PEM or base64 encoded)
- `TLS_FORCE_HTTPS` - Force redirect HTTP to HTTPS (default: `false`).
  When `true`, all `HTTP_PORT` requests get a `301` redirect to
  `https://${HTTP_HOST}:${HTTPS_PORT}`.

**Ports.** `HTTPS_PORT` defaults to `443`. Binding to a privileged port (≤1024)
needs `CAP_NET_BIND_SERVICE` — the systemd unit shipped by `gameapctl panel
install` already grants it. In Docker, expose the port and run as root or set
`--cap-add=NET_BIND_SERVICE`.

#### Static TLS example

```bash
HTTP_HOST=panel.example.com
HTTP_PORT=80
HTTPS_PORT=443
TLS_CERT_FILE=/etc/ssl/gameap/fullchain.pem
TLS_KEY_FILE=/etc/ssl/gameap/privkey.pem
TLS_FORCE_HTTPS=true
```

### Let's Encrypt (ACME) Configuration

GameAP embeds the [`go-acme/lego`](https://github.com/go-acme/lego) ACME client
and can manage Let's Encrypt certificates in-process — no external `certbot`,
no nginx, no cron. Renewal runs in a background goroutine and the certificate
is hot-swapped via `tls.Config.GetCertificate`, so renewals never restart the
HTTPS listener.

Two challenge solvers are supported:

| Solver    | Wildcards | Network requirement                | Best for                 |
|-----------|-----------|-------------------------------------|--------------------------|
| `http-01` | ❌ no     | Inbound TCP/80 reachable from LE   | Single-domain panels     |
| `dns-01`  | ✅ yes    | API access to your DNS provider    | Wildcards, firewalled VMs |

If the initial issuance fails (LE unreachable, DNS provider misconfigured, …)
the panel exits with code 1 — there is no silent fallback to plain HTTP. Run
against the LE staging endpoint while iterating on configuration.

#### ACME environment variables

- `ACME_ENABLED` - Enable in-process ACME (default: `false`)
- `ACME_CHALLENGE_TYPE` - `http-01` or `dns-01` (default: `http-01`)
- `ACME_EMAIL` - Account email registered with Let's Encrypt (required)
- `ACME_DOMAINS` - Comma-separated list of domains. Wildcards (`*.example.com`)
  require `dns-01`.
- `ACME_DIRECTORY_URL` - ACME directory endpoint (default: Let's Encrypt
  production; switch to `https://acme-staging-v02.api.letsencrypt.org/directory`
  for testing)
- `ACME_DNS_PROVIDER` - DNS provider name when `ACME_CHALLENGE_TYPE=dns-01`
  (currently built-in: `cloudflare`)
- `ACME_RENEWAL_THRESHOLD` - Renew when the cert has less than this duration
  remaining (default: `720h` = 30 days)
- `ACME_RENEWAL_CHECK_INTERVAL` - How often the background loop inspects the
  certificate (default: `12h`)
- `ACME_PROPAGATION_TIMEOUT` - Maximum wait for DNS propagation during
  `dns-01` (default: `180s`)
- `ACME_STORAGE_PATH` - Subdirectory under the `files.FileManager` root used
  to persist the ACME account and certificate material (default: `acme`).
  With `FILES_DRIVER=local`, this resolves to
  `${FILES_LOCAL_BASE_PATH}/acme/`. With `FILES_DRIVER=s3`, it lives in the
  configured bucket — which is what enables multi-instance deployments.

#### HTTP-01 example

`http-01` requires Let's Encrypt to reach `http://${ACME_DOMAINS}/.well-known/acme-challenge/...`.
That means the panel must listen on port 80 (or have a reverse proxy that
forwards `/.well-known/acme-challenge/*` to it).

```bash
HTTP_HOST=panel.example.com
HTTP_PORT=80
HTTPS_PORT=443
TLS_FORCE_HTTPS=true

ACME_ENABLED=true
ACME_CHALLENGE_TYPE=http-01
ACME_EMAIL=ops@example.com
ACME_DOMAINS=panel.example.com
```

The `/.well-known/acme-challenge/{token}` route is registered ahead of the
SPA fallback automatically; you do not need to configure it.

#### DNS-01 + Cloudflare example

`dns-01` does not need port 80 to be reachable. Lego writes a TXT record at
`_acme-challenge.<domain>` via your DNS provider's API. The Cloudflare
provider reads its credentials directly from the environment.

```bash
HTTP_HOST=panel.example.com
HTTPS_PORT=443
TLS_FORCE_HTTPS=true

ACME_ENABLED=true
ACME_CHALLENGE_TYPE=dns-01
ACME_EMAIL=ops@example.com
ACME_DOMAINS=*.example.com,example.com
ACME_DNS_PROVIDER=cloudflare

# Read by lego's cloudflare provider — scope the token to the relevant zone.
CLOUDFLARE_DNS_API_TOKEN=cf-token-with-Zone-DNS-Edit-permission
```

Other DNS providers will be added over time — open an issue if you need one.

#### Staging vs. production

Let's Encrypt enforces aggressive rate limits on the production directory
(5 duplicate certs / week, 50 certs / week / registered domain, …). While
testing, point at the staging endpoint to avoid getting locked out:

```bash
ACME_DIRECTORY_URL=https://acme-staging-v02.api.letsencrypt.org/directory
```

Browsers will warn about the staging certificate — that is expected. Switch
back to the default production URL once the flow works end to end.

#### Multi-instance deployments

A single panel instance is fully self-contained. For horizontally scaled
deployments:

- **`dns-01`** is the supported path. Set `CACHE_DRIVER=redis` (the Redis
  client is also used as the distributed locker for renewals) and
  `FILES_DRIVER=s3` so every replica reads the same certificate. Only one
  replica at a time talks to LE; the rest pick up the new certificate from
  shared storage.
- **`http-01`** in a multi-instance setup needs sticky session affinity for
  `/.well-known/acme-challenge/*` at the load balancer (the challenge token
  lives in memory on the instance that received the `Present` call).
  `dns-01` is usually less hassle.

#### gameapctl helper

Editing `config.env` by hand is fine, but the friendlier path is:

```bash
gameapctl panel letsencrypt setup
gameapctl panel letsencrypt disable
```

`setup` is an interactive wizard (also accepts `--challenge`, `--domains`,
`--email`, `--dns-provider`, `--staging`, `--env KEY=VALUE`,
`--non-interactive` flags) that writes the variables and restarts the
`gameap` systemd service. `disable` clears the `ACME_*` keys.

#### Status endpoint

Admin-only `GET /api/admin/letsencrypt/status` returns the current ACME
state, useful for monitoring dashboards and the `gameapctl` polling logic:

```json
{
  "enabled": true,
  "state": "active",
  "challenge_type": "http-01",
  "domains": ["panel.example.com"],
  "dns_provider": "",
  "not_before": "2026-04-01T00:00:00Z",
  "not_after":  "2026-06-30T00:00:00Z",
  "last_renewal_at": "2026-04-01T00:00:00Z",
  "next_renewal_check_at": "2026-04-01T12:00:00Z"
}
```

`state` is one of `disabled`, `pending`, `active`, `renewing`, `failed`.

### Database Configuration

- `DATABASE_DRIVER` - Database driver (required, options: `mysql`, `postgres`, `sqlite`, `inmemory`)
- `DATABASE_URL` - Database connection URL (required)
- `DATABASE_CONNECT_TIMEOUT` - How long to retry the initial database connect at startup (default: `30s`)
  - MySQL: `username:password@tcp(host:port)/database?parseTime=true`
  - PostgreSQL: `postgres://username:password@host:port/database?sslmode=disable`
  - SQLite: `file:path/to/database.db?_busy_timeout=5000&_journal_mode=WAL&cache=shared` (parameters recommended for production)
  - Inmemory: For `inmemory`, this can be left empty.

### Security Configuration

- `ENCRYPTION_KEY` - Encryption key for sensitive data
- `AUTH_SECRET` - Secret key for PASETO/JWT token generation (if not set, uses `ENCRYPTION_KEY`)
- `AUTH_SERVICE` - Authentication service type (default: `paseto`)
- `AUTH_ALLOW_WEAK_PASSWORDS` - When `true`, disables the embedded common-password blocklist check used at registration / password change (OWASP ASVS §2.1.7). Default: `false`. Enabling this logs a startup warning; length checks (min 12 / max 128) still apply.
- `AUTH_SSO_TICKET_TTL` - Lifetime of a single-use SSO login ticket minted for another user, exchanged by external systems (a billing panel) for a logged-in customer session. Hard-capped at `120s` by the issuing handler. Default: `60s`. Behind a load balancer this requires a shared `CACHE_DRIVER` (`redis`/`postgres`): the ticket is minted on one instance and redeemed by the browser on another, so the `memory` driver cannot see it.

#### Single sign-on for a customer who administers their own panel

In the common "one panel per customer" deployment the customer *is* the panel's
administrator, seeded from `ADMIN_LOGIN` / `ADMIN_EMAIL` / `ADMIN_PASSWORD`. To
offer them the same "open my game panel" button, the external system signs in
once with that password and calls `POST /api/tokens` to mint a personal access
token carrying `admin:user:sso`; the password is not needed afterwards. A token
is always bound to whoever created it, so the token it now holds belongs to that
administrator.

That ownership is what makes the administrator case safe to allow at all: a
ticket naming an administrator is minted only when the target is the token's own
owner, so a stolen token reaches exactly the identity it already spoke for and no
other. A ticket for any other administrator is refused.

The customer lands in the panel on the first click, before they have set up
anything. From there the ordinary admin-MFA policy takes over, the same one a
password login applies:

- while the grace window lasts (`AUTH_MFA_HARD_FAIL_DAYS`, default 30 days) the
  ticket yields a normal session and the panel asks them to enable two-factor
  authentication;
- once the window closes the ticket yields a session scoped to the 2FA-enrollment
  screens and nothing else, until they enrol;
- afterwards the ticket only ever yields a second-factor challenge, so the
  customer's authenticator is needed to get in.

Two consequences worth planning for. Until that second factor exists, a token
stolen out of the external system is enough to reach an administrative session —
which is what bounds the exposure to the grace window, and why setting
`AUTH_REQUIRE_MFA_FOR_ADMINS=false` leaves it open indefinitely. And once it
exists, **disabling** it or regenerating recovery codes needs the account
password, which in this deployment only the external system knows: a customer who
loses their authenticator has to come back through you.

### RBAC Configuration

- `RBAC_CACHE_TTL` - Role-based access control cache TTL (default: `30s`)

### Cache Configuration

- `CACHE_DRIVER` - Cache driver (options: `memory`, `redis`, `postgres`, default: `memory`)

#### Redis Cache

Used when `CACHE_DRIVER` is set to `redis`.

- `CACHE_REDIS_ADDR` - Redis server address (default: `localhost:6379`)
- `CACHE_REDIS_PASSWORD` - Redis password
- `CACHE_REDIS_DB` - Redis database number (default: `0`)

#### Cache TTL

- `CACHE_TTL_RBAC` - Cache TTL for RBAC data (default: `24h`)
- `CACHE_TTL_GAMES` - Cache TTL for games (default: `48h`)
- `CACHE_TTL_NODES` - Cache TTL for nodes (default: `24h`)
- `CACHE_TTL_USERS` - Cache TTL for users (default: `6h`)
- `CACHE_TTL_PERSONAL_TOKENS` - Cache TTL for personal tokens (default: `24h`)
- `CACHE_TTL_SERVER_SETTINGS` - Cache TTL for server settings (default: `12h`)

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

#### Chunked Upload Sessions

Used by the resumable file-manager upload endpoints
(`/api/file-manager/{server}/upload/sessions`).

- `FILES_UPLOAD_CHUNK_SIZE` - Server-decided chunk size returned to clients. Accepts plain bytes (`8388608`) or human-readable sizes with binary suffixes (`8M`, `8MB`, `8MiB`, `1000KB`, `2GB`). Default: `8M`.
- `FILES_UPLOAD_SESSION_TTL` - How long an in-progress upload session lives before janitor reclaims it (default: `24h`)
- `FILES_UPLOAD_MAX_CHUNKS` - Hard cap on chunks per file; bounds the maximum file size to `FILES_UPLOAD_CHUNK_SIZE × FILES_UPLOAD_MAX_CHUNKS` (default: `1000000`)
- `FILES_UPLOAD_JANITOR_INTERVAL` - How often the background janitor scans for expired upload sessions (default: `1h`)

### Legacy Configuration

- `LEGACY_PATH` - Path to legacy GameAP installation (default: `/var/www/gameap/`)
- `LEGACY_ENV_PATH` - Path to legacy .env file (default: `/var/www/gameap/.env`)

### Global API Configuration

- `GLOBAL_API_URL` - Global GameAP API URL for game updates (default: `https://api.gameap.com`)

### Logger Configuration

- `LOGGER_LEVEL` - Log level (options: `debug`, `info`, `warn`, `error`, default: `info`)
- `LOGGER_LOG_DB_QUERIES` - Enable database query logging (default: `false`)

### UI Configuration

- `DEFAULT_LANGUAGE` - Default UI language code

### Plugins Configuration

Every plugin setting used to be spelled `PLUGIN_…`; only the loader switches
carried the `PLUGINS_` prefix. All of them now use `PLUGINS_`, and the two
compilation-cache switches became `PLUGINS_RUNTIME_CACHE_ENABLED` and
`PLUGINS_RUNTIME_CACHE_DIR` so that `PLUGINS_CACHE_` belongs to the
`gameap-cache` host library alone. The former names are still accepted and
translated at startup, with a warning naming the replacement; they will be
removed in a future release.

- `PLUGINS_DISABLED` - Disable plugins support (default: `false`)
- `PLUGINS_AUTOLOAD` - Comma-separated wasm files from the plugins directory to register at startup
- `PLUGINS_STRICT_LOAD` - Refuse to start when any plugin fails to load (default: `false`; a broken plugin is marked with status `error` and skipped)
- `PLUGINS_RUNTIME_CACHE_ENABLED` - Cache compiled wasm between loads (default: `true`)
- `PLUGINS_RUNTIME_CACHE_DIR` - Local directory for the compilation cache so panel restarts do not recompile every plugin (default: in-memory only)
- `PLUGINS_RUNTIME_MAX_MEMORY` - Linear memory cap per plugin module (default: `256M`, `0` = wazero default of 4 GiB)
- `PLUGINS_RUNTIME_MAX_MODULE_SIZE` - Maximum wasm file size accepted for install and load (default: `128M`, `0` = unlimited)
- `PLUGINS_PERMISSIONS_ENFORCE` - Apply the recorded plugin permission grants (default: `false`; a future release will default it to `true`). While off, grants are recorded, shown and editable but every check passes, giving plugin developers time to declare theirs. Set it identically on every panel instance; `GET /api/admin/plugins/loaded` reports the answering instance's value as `permissions_enforced`
- `PLUGINS_PERMISSIONS_CACHE_TTL` - How long granted permissions stay cached in the instance's memory (default: `30s`, `0` = read the plugin record on every check). A grant change is announced over pub/sub and drops the cache on every instance, so this only bounds the drift while the broker is unreachable
- `PLUGINS_RECOVERY_ENABLED` - Reload plugins the runtime disabled (guest call timeout, guest exit) with exponential backoff (default: `true`)
- `PLUGINS_RECOVERY_INITIAL_DELAY` - Wait before the first automatic reload (default: `30s`; doubles every attempt)
- `PLUGINS_RECOVERY_MAX_DELAY` - Cap for the backoff (default: `10m`)
- `PLUGINS_RECOVERY_MAX_ATTEMPTS` - Consecutive reloads before the plugin stays in status `error` until an operator reloads it (default: `5`)
- `PLUGINS_SCHEDULER_MIN_INTERVAL` - Shortest interval a plugin may register a scheduled task with (default: `1s`)
- `PLUGINS_SCHEDULER_MAX_TASKS_PER_PLUGIN` - Scheduled tasks one plugin may register (default: `32`)
- `PLUGINS_SCHEDULER_CALL_TIMEOUT` - Budget for one task handler call when the task sets no timeout of its own (default: `60s`)
- `PLUGINS_SCHEDULER_MAX_CALL_TIMEOUT` - Ceiling for a per-task timeout override (default: `5m`)
- `PLUGINS_SCHEDULER_MAX_RETRIES` / `PLUGINS_SCHEDULER_MAX_RETRY_DELAY` / `PLUGINS_SCHEDULER_MAX_JITTER` - Caps on the retry policy a task may ask for (default: `10` / `10m` / `30s`)
- `PLUGINS_SCHEDULER_REFRESH_INTERVAL` - How often task definitions are re-read to pick up registrations made on other instances (default: `30s`)
- `PLUGINS_NODEFS_MAX_INLINE` - Largest file a plugin may download or upload in one `gameap-nodefs` call (default: `32M`, `0` = unlimited)
- `PLUGINS_NODEFS_PATH_POLICY` - Where plugins may point `gameap-nodefs`, `gameap-nodecmd` working directories and file references: `unrestricted` (default, anything the daemon permits), `node_workpath` (inside the node's work path) or `server_dirs` (inside a game server directory on that node). Paths with `..` segments are refused in every mode
- `PLUGINS_NODEFS_ALLOWED_PATHS` - Comma-separated absolute roots allowed on top of a restricted path policy (e.g. `/opt/steamcmd`)
- `PLUGINS_SYNC_DISABLED` - Stop reconciling plugins against the database on this instance; each instance then only applies its own changes (default: `false`)
- `PLUGINS_SYNC_REFRESH_INTERVAL` - How often an instance re-reads the plugin table to pick up changes made elsewhere; pubsub hints apply them sooner (default: `60s`)
- `PLUGINS_SYNC_MIN_BACKOFF` / `PLUGINS_SYNC_MAX_BACKOFF` - Retry window for a plugin this instance could not load (default: `15s` / `15m`)
- `PLUGINS_STORAGE_MAX_KEYS_PER_PLUGIN` - Entries one plugin may keep in `gameap-storage` (default: `10000`)
- `PLUGINS_STORAGE_MAX_VALUE` - Largest single `gameap-storage` payload (default: `1M`)
- `PLUGINS_STORAGE_MAX_TOTAL` - Sum of all `gameap-storage` payloads of one plugin (default: `64M`)
- `PLUGINS_CACHE_MAX_VALUE` - Largest single `gameap-cache` value (default: `1M`, `0` = unlimited); every plugin has its own cache namespace
- `PLUGINS_SECRETS_MAX_KEYS_PER_PLUGIN` - Secrets one plugin may keep in `gameap-secrets` (default: `64`)
- `PLUGINS_SECRETS_MAX_VALUE` - Largest plaintext of a single secret (default: `8K`)
- `PLUGINS_SECRETS_REQUIRE_ENCRYPTION` - Refuse `gameap-secrets` writes while `ENCRYPTION_KEY` is unset instead of keeping them in plaintext (default: `true`)
- `PLUGINS_HTTP_MAX_TIMEOUT` - Ceiling for the per-request timeout a plugin asks for in `gameap-http`; a longer one is clamped (default: `30s`)
- `PLUGINS_NET_MAX_TIMEOUT` - Ceiling for one `gameap-net` operation - the dial plus every read and write (default: `10s`)
- `PLUGINS_NET_READ_BUFFER` - Largest single `gameap-net` receive a plugin may request (default: `64K`)
- `PLUGINS_RATELIMIT_NODECMD_RPS` / `PLUGINS_RATELIMIT_NODECMD_BURST` - Per-plugin token bucket for `gameap-nodecmd` (default: `5` / `20`; RPS `0` = no limit)
- `PLUGINS_RATELIMIT_SERVERCONTROL_RPS` / `PLUGINS_RATELIMIT_SERVERCONTROL_BURST` - Server control, daemon task creation, server and server-setting writes (default: `5` / `20`)
- `PLUGINS_RATELIMIT_NODEFS_RPS` / `PLUGINS_RATELIMIT_NODEFS_BURST` - Every `gameap-nodefs` operation (default: `50` / `200`)
- `PLUGINS_RATELIMIT_HTTP_RPS` / `PLUGINS_RATELIMIT_HTTP_BURST` - `gameap-http` requests (default: `20` / `50`)
- `PLUGINS_RATELIMIT_RBAC_RPS` / `PLUGINS_RATELIMIT_RBAC_BURST` - `gameap-rbac` calls (default: `10` / `50`)
- `PLUGINS_RATELIMIT_SSH_RPS` / `PLUGINS_RATELIMIT_SSH_BURST` - Every `gameap-ssh` call, polling a running command included (default: `20` / `60`)

Rate limits are per panel instance; a refused call answers with a `rate limited: ...` error in the host
response and the plugin keeps running. Plugin grants (`manage_servers`, `node_commands`, `files`,
`files_read`, `listen_events`, `manage_rbac`, `secrets`, `ssh`) are managed per plugin in the admin UI or through
`PUT /api/admin/plugins/{id}/permissions`; see `pkg/plugin/README.md`.

#### Plugins across instances

Several panel instances sharing one database keep their plugins in step on their own: the plugin
table is the desired state, and every instance applies an install, update, uninstall, reload or
permission change made on any other instance — immediately when the instances share
a pubsub (`PUBSUB_DRIVER=redis`), otherwise on the next `PLUGINS_SYNC_REFRESH_INTERVAL` pass. Set
`PUBSUB_INSTANCE_ID` to a stable, distinct value per instance so log lines, audit records and
`NODE_ONLINE` / `NODE_OFFLINE` plugin events name the instance. Plugins installed from the store are
re-downloaded by an instance that lacks the file (and verified against the recorded checksum); plugins
uploaded from a file are only recoverable when `FILES_DRIVER` points at shared storage such as S3.
`GET /api/admin/plugins/loaded` always describes the answering instance (`loaded`, `sync`), while
`status` / `error` are the shared record.

### Metrics

- `METRICS_TOKEN` - Bearer token for the Prometheus scrape endpoint `GET /metrics`; empty (default) leaves the endpoint unregistered. The endpoint exposes `gameap_plugin_*` metrics (host/guest calls, refusals, events, disables, memory) plus the Go runtime and process collectors.

### Plugin Capabilities Configuration

Bounds on what installed plugins may do through the host libraries. Defaults
are strict: a compromised plugin must not be able to pivot from the panel into
private networks or cloud metadata endpoints.

- `PLUGINS_HTTP_BLOCK_PRIVATE_IPS` - Refuse plugin HTTP requests to loopback/private/link-local addresses (default: `true`)
- `PLUGINS_HTTP_ALLOWED_SCHEMES` - Comma-separated URL schemes plugins may fetch (default: `https`)
- `PLUGINS_HTTP_ALLOWED_HOSTS` - Hosts exempt from the private-IP block; empty leaves the blocklist as the only gate
- `PLUGINS_HTTP_MAX_TIMEOUT` - Ceiling for a plugin's own request timeout (default: `30s`)
- `PLUGINS_HTTP_MAX_REDIRECTS` - Redirect limit (default: `5`)
- `PLUGINS_HTTP_RESPONSE_HEADER_ALLOWLIST` - Extra response headers passed back to the plugin on top of the built-in list (`Content-Type`, `Content-Length`, `Content-Encoding`, `Last-Modified`, `ETag`, `Cache-Control`, `Date`, `Location`, `Expires`). Credential-bearing headers such as `Set-Cookie` and `WWW-Authenticate` are never passed through
- `PLUGINS_NET_ENABLED` - Enable the plugin socket library used by custom RCON/Query protocols (default: `true`)
- `PLUGINS_NET_BLOCK_PRIVATE_IPS` - Refuse game-server connections to private addresses (default: `false`; self-hosted servers commonly live on private networks)
- `PLUGINS_NET_ALLOWED_HOSTS` - Hosts exempt from that block
- `PLUGINS_NET_MAX_TIMEOUT` - Ceiling for a single read/write (default: `10s`)
- `PLUGINS_NET_READ_BUFFER` - Cap on a single read (default: `64K`)
- `PLUGINS_NET_MAX_CONNECTIONS` - Open connections per plugin (default: `8`)

SSH is the one capability where a plugin names its own target, so a machine can
be reached before it has a daemon. It is off until an operator turns it on:

- `PLUGINS_SSH_ENABLED` - Enable the gameap-ssh host library (default: `false`)
- `PLUGINS_SSH_BLOCK_PRIVATE_IPS` - Refuse SSH to loopback/private/link-local addresses (default: `true`). Cloud-metadata addresses are blocked regardless
- `PLUGINS_SSH_ALLOWED_HOSTS` - Hosts exempt from that block, for panels whose dedicated servers live on a private network
- `PLUGINS_SSH_MAX_CONNECTIONS` - Open SSH connections per plugin (default: `8`)
- `PLUGINS_SSH_MAX_OPERATIONS` - Concurrently running commands per plugin (default: `16`)
- `PLUGINS_SSH_CONNECT_TIMEOUT` - Budget for dial, handshake and authentication (default: `30s`)
- `PLUGINS_SSH_MAX_EXEC_TIMEOUT` - Ceiling for one remote command (default: `30m`)
- `PLUGINS_SSH_IDLE_TIMEOUT` - Close a connection nothing has run on for this long (default: `10m`)
- `PLUGINS_SSH_MAX_OUTPUT_BYTES` - Captured stdout/stderr per command; the head is kept (default: `1048576`)
- `PLUGINS_SSH_MAX_STDIN_BYTES` - Cap on what a plugin may pipe into a command (default: `1048576`)
- `PLUGINS_SSH_ALLOW_ACCEPT_ANY_HOST_KEY` - Permit the `accept_any` host key policy (trust-on-first-use); disable to force pinned keys (default: `true`)
- `PLUGINS_SSH_OPERATION_RETENTION` - Keep a finished command (with its captured output) readable for late polls (default: `10m`)
- `PLUGINS_SSH_MAX_RETAINED_OPERATIONS` - Finished commands kept per plugin; the oldest are evicted first (default: `64`)
- `PLUGINS_SSH_KEEPALIVE_INTERVAL` - Pacing of liveness probes on open connections; the effective sweep is floored at one second (default: `30s`)
- `PLUGINS_SSH_COMPLETION_CALL_TIMEOUT` - Budget for one completion callback into the plugin (default: `30s`)
- `PLUGINS_SSH_BUSY_RETRY_DELAY` - Pause between completion callback retries while the plugin is busy (default: `2s`)
- `PLUGINS_SSH_BUSY_RETRIES` - Completion callback retries before the callback is dropped (default: `5`)

### Plugin Store Configuration

- `PLUGINS_STORE_URL` - GameAP plugin store URL (default: `https://plugins.gameap.dev/api`)
- `PLUGINS_STORE_LICENSE_KEY` - License key for plugin store

### CAPTCHA Configuration

Optional CAPTCHA protection for the login endpoint (`POST /api/auth/login`)
against automated credential stuffing. Disabled until `CAPTCHA_PROVIDER` is
set. The login form discovers the provider and public site key through
`GET /api/config/public`; the secret key is never exposed there. The token
is sent as the `captcha` field of the login request and is verified before
the user lookup.

- `CAPTCHA_PROVIDER` - CAPTCHA provider (options: `recaptcha_v2`, `recaptcha_v3`, `turnstile`; empty disables CAPTCHA, default: empty)
- `CAPTCHA_SITE_KEY` - Public site key for the provider widget (safe to expose to browsers)
- `CAPTCHA_SECRET_KEY` - Server-side secret key used to verify tokens (kept server-side)
- `CAPTCHA_MIN_SCORE` - Pass threshold for reCAPTCHA v3 only, `0.0`–`1.0`; ignored by the checkbox/Turnstile providers (default: `0.5`)
- `CAPTCHA_FAIL_OPEN` - Allow login when the provider's verify call itself fails (network/5xx). Default `false` (fail-closed: a verification outage blocks login with `503`)
- `CAPTCHA_VERIFY_URL` - Override the provider's `siteverify` endpoint (egress proxies, testing). Empty uses the provider default

### HTTP Security Headers Configuration

Global response headers emitted on every HTTP/HTTPS response — HSTS,
`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy` and a
Content-Security-Policy. Secure defaults are on out-of-the-box; every directive
is env-overridable so deployments with plugins or unusual reverse-proxy setups
can extend the policy without patching the binary.

The generated CSP automatically allow-lists exactly the origins required by
the configured CAPTCHA provider (Google reCAPTCHA scripts/iframes for
`recaptcha_v2`/`recaptcha_v3`; `challenges.cloudflare.com` for `turnstile`); with
CAPTCHA disabled no third-party origins appear in the policy.

HSTS is only emitted on TLS requests (`r.TLS != nil`, `X-Forwarded-Proto: https`
from a trusted reverse proxy, or `TLS_FORCE_HTTPS=true`) so plain-HTTP dev
sessions are never poisoned.

- `SECURITY_HEADERS_ENABLED` - Master switch for all security headers (default: `true`)
- `SECURITY_HSTS_ENABLED` - Toggle `Strict-Transport-Security` emission (default: `true`)
- `SECURITY_HSTS_MAX_AGE` - HSTS max-age in seconds (default: `31536000`, i.e. 1 year)
- `SECURITY_HSTS_INCLUDE_SUBDOMAINS` - Append `includeSubDomains` (default: `false` — turn on only after every subdomain is HTTPS)
- `SECURITY_HSTS_PRELOAD` - Append `preload` (default: `false` — only enable when intending to submit to the HSTS preload list)
- `SECURITY_CONTENT_TYPE_OPTIONS` - Emit `X-Content-Type-Options: nosniff` (default: `true`)
- `SECURITY_FRAME_OPTIONS` - `X-Frame-Options` value, empty to omit (default: `SAMEORIGIN`)
- `SECURITY_REFERRER_POLICY` - `Referrer-Policy` value, empty to omit (default: `strict-origin-when-cross-origin`)
- `SECURITY_CSP_ENABLED` - Emit a Content-Security-Policy header (default: `true`)
- `SECURITY_CSP_REPORT_ONLY` - Use `Content-Security-Policy-Report-Only` instead of enforcing — for staged rollout (default: `false`)
- `SECURITY_CSP_POLICY` - Verbatim CSP override; when set, the generated policy and the additive `SECURITY_CSP_EXTRA_*` lists are ignored (default: empty)
- `SECURITY_CSP_REPORT_URI` - Endpoint appended as `report-uri` to receive CSP violation reports (default: empty)
- `SECURITY_CSP_EXTRA_SCRIPT_SRC` - Comma-separated extra origins appended to `script-src` (e.g. a plugin's CDN)
- `SECURITY_CSP_EXTRA_STYLE_SRC` - Comma-separated extra origins appended to `style-src`
- `SECURITY_CSP_EXTRA_CONNECT_SRC` - Comma-separated extra origins appended to `connect-src`
- `SECURITY_CSP_EXTRA_IMG_SRC` - Comma-separated extra origins appended to `img-src`
- `SECURITY_CSP_EXTRA_FRAME_SRC` - Comma-separated extra origins appended to `frame-src`
- `SECURITY_CSP_EXTRA_FONT_SRC` - Comma-separated extra origins appended to `font-src`

### Example Configuration

```bash
# Server
HTTP_HOST=panel.example.com
HTTP_PORT=8025
HTTPS_PORT=443

# --- TLS: pick ONE of (a), (b) or (c). Leave all commented out for plain HTTP. ---

# (a) Static cert files
# TLS_CERT_FILE=/etc/ssl/gameap/fullchain.pem
# TLS_KEY_FILE=/etc/ssl/gameap/privkey.pem
# TLS_FORCE_HTTPS=true

# (b) ACME / Let's Encrypt — HTTP-01 (port 80 must be reachable from LE)
# HTTP_PORT=80
# TLS_FORCE_HTTPS=true
# ACME_ENABLED=true
# ACME_CHALLENGE_TYPE=http-01
# ACME_EMAIL=ops@example.com
# ACME_DOMAINS=panel.example.com

# (c) ACME / Let's Encrypt — DNS-01 + Cloudflare (supports wildcards)
# TLS_FORCE_HTTPS=true
# ACME_ENABLED=true
# ACME_CHALLENGE_TYPE=dns-01
# ACME_EMAIL=ops@example.com
# ACME_DOMAINS=*.example.com,example.com
# ACME_DNS_PROVIDER=cloudflare
# CLOUDFLARE_DNS_API_TOKEN=cf-token-with-Zone-DNS-Edit-permission
# Use the staging endpoint while iterating to avoid LE rate limits:
# ACME_DIRECTORY_URL=https://acme-staging-v02.api.letsencrypt.org/directory

# Database
DATABASE_DRIVER=mysql
DATABASE_URL=gameap:password@tcp(localhost:3306)/gameap?parseTime=true

# Security
ENCRYPTION_KEY=your-secure-encryption-key-here
AUTH_SECRET=your-secure-auth-secret-here
AUTH_SERVICE=paseto

# Cache
CACHE_DRIVER=memory
# For Redis cache (also enables the distributed lock used by ACME renewals):
# CACHE_DRIVER=redis
# CACHE_REDIS_ADDR=localhost:6379

# File Storage
FILES_DRIVER=local
FILES_LOCAL_BASE_PATH=/var/lib/gameap/files
# For multi-instance deployments switch to S3 so every replica sees the same
# ACME storage:
# FILES_DRIVER=s3
# FILES_S3_ENDPOINT=https://s3.example.com
# FILES_S3_BUCKET=gameap
# FILES_S3_ACCESS_KEY_ID=...
# FILES_S3_SECRET_ACCESS_KEY=...

# Legacy
LEGACY_PATH=/var/www/gameap/

# Global API
GLOBAL_API_URL=https://api.gameap.com

# Logger
LOGGER_LEVEL=info

# Plugins
# PLUGINS_DISABLED=false

# Plugin capabilities — defaults are strict; SSH is off until enabled
# PLUGINS_SSH_ENABLED=false
# PLUGINS_SSH_BLOCK_PRIVATE_IPS=true     # cloud metadata is blocked either way
# PLUGINS_SSH_ALLOWED_HOSTS=node1.internal,node2.internal
# PLUGINS_SSH_MAX_EXEC_TIMEOUT=30m

# Plugin Store
# PLUGINS_STORE_URL=https://plugins.gameap.dev/api
# PLUGINS_STORE_LICENSE_KEY=your-license-key

# CAPTCHA (login protection) — leave CAPTCHA_PROVIDER empty to disable
# CAPTCHA_PROVIDER=turnstile          # recaptcha_v2 | recaptcha_v3 | turnstile
# CAPTCHA_SITE_KEY=your-public-site-key
# CAPTCHA_SECRET_KEY=your-server-side-secret-key
# CAPTCHA_MIN_SCORE=0.5               # reCAPTCHA v3 only
# CAPTCHA_FAIL_OPEN=false             # true = allow login if the provider is unreachable

# HTTP security headers — secure defaults are on; uncomment to tune
# SECURITY_HSTS_INCLUDE_SUBDOMAINS=true    # only after every subdomain is HTTPS
# SECURITY_HSTS_PRELOAD=true               # only if submitting to hstspreload.org
# SECURITY_CSP_REPORT_ONLY=true            # stage CSP without enforcing first
# SECURITY_CSP_REPORT_URI=https://csp.example.com/report
# SECURITY_CSP_EXTRA_SCRIPT_SRC=https://cdn.plugin.example
```