# pluginsync

Keeps every panel instance's loaded plugins in step with the `plugins` table.

Installing, updating, removing, reloading or reconfiguring a plugin changes
the runtime of exactly one instance — the one that handled the HTTP request.
Without this package the other replicas keep serving the old module (or none
at all) until they are restarted, and with them the plugin's HTTP routes,
frontend bundle, server abilities, RCON/Query protocol registrations and
event subscriptions.

## How it works

The database is the desired state. The reconciler compares it against what
the loader currently runs on this instance and closes the gap through the
loader's `ApplyRecord` / `UnloadRecord`, so the loader stays the single
owner of plugin lifecycle (per-plugin lifecycle locks, ID registration,
fingerprints, lifecycle events).

A pub/sub message on `gameap:plugin:sync` carries **no state** — a plugin id
and the action, for logging. It only wakes the reconciler, which re-reads the
database and decides for itself. That is what makes the feature survive the
transport it runs on: the pub/sub drivers are at most once with no persistence
and no replay, so a message can be lost, duplicated or delivered out of order
without changing the result. The worst a lost message costs is one
`PLUGIN_SYNC_REFRESH_INTERVAL` of staleness (the periodic pass is jittered so
replicas do not hit the database in lockstep).

The same property makes the "who sent it" question irrelevant — an instance
handles its own hints too, because a pass is idempotent, and filtering on the
sender would break outright when `PUBSUB_INSTANCE_ID` is left unset and every
replica shares the same identifier.

Passes are single-flight: a hint during a pass schedules one more pass, never
a concurrent one. `Start` runs the first pass synchronously after the loader's
`LoadAll` and the bootstrap subscription refresh.

## Decision table

Each pass reads every plugin row (**on a read failure it touches nothing**,
because an unreachable database must not drain the plugins of every instance
that notices) and decides per plugin from the row status and the local
runtime state:

| Row | Loaded here | Action |
|---|---|---|
| gone (the id was seen in an earlier successful read), or `disabled` | present | `UnloadRecord` (trigger `sync`) and drop its archive callbacks |
| `updating`, or `disabled` and absent | — | nothing: an operator's multi-step operation is in flight elsewhere |
| `active` | present, enabled, same fingerprint | nothing |
| `active` | present but a different fingerprint, or disabled at runtime by the supervisor | `ApplyRecord` (replaces the module) |
| `active` | absent | `ApplyRecord`, retried with backoff (`PLUGIN_SYNC_MIN_BACKOFF` .. `PLUGIN_SYNC_MAX_BACKOFF`); a fingerprint change resets the backoff |
| `error` | present | nothing, unless the fingerprint differs → `ApplyRecord` |
| `error` | absent | `ApplyRecord` once per fingerprint, or again after the file was repaired — no timed retry (that is the supervisor's `PLUGIN_RECOVERY_*` contract) |

`ApplyRecord` answers `ErrPluginHeld` while an admin handler holds the plugin
(update, uninstall, configuration change); that is contention, not a failure:
the plugin is retried after a short flat delay (10s) and never counted as
failed.

Event subscriptions are rebuilt once per pass when membership changed **or**
a running plugin's `listen_events` grant changed since the last pass —
`PUT /api/admin/plugins/{id}/permissions` only refreshes its own instance.
Reloads performed by the reconciler are audited as `plugin.reloaded` with
`trigger=sync`; every pass is counted in
`gameap_plugin_sync_passes_total{result}` and the number of plugins still
out of step in `gameap_plugin_sync_pending`.

## Reload fingerprint

`internalplugin.Fingerprint(row)` = sha256 of `version`, `filename`,
`checksum`, the canonical JSON of `config` and `generation`.

- **`config`** is in: it reaches the guest only through `Initialize`, so a
  change needs a reload. Secrets are compared as their stored envelopes, so the
  fingerprint never sees plaintext; schema defaults are overlaid at load and
  never persisted, so a plugin update that ships a new default changes the
  fingerprint through `checksum`, not through the row's config.
- **`generation`** is in: an operator reload (`POST .../reload` or a
  configuration save) bumps it, which is how "reload on instance A" becomes a
  restart on every instance. Supervisor recoveries do not bump it — a guest
  timeout on one instance is not a reason to restart the others.
- **`allowed_permissions`** is out: grants are re-read on every host call,
  and `listen_events` is handled by the subscription refresh above.
- **`priority`**, `status`, timestamps and `config_schema` (derived from the
  wasm, covered by `checksum`) are out.

## Failure handling

A failed load is recorded in memory with an exponential backoff and reported
per plugin on `GET /api/admin/plugins/loaded` as
`sync: {state, error, failures, last_attempt_at, next_retry_at}`; a plugin
this instance could not load is additionally shown with `status=error` and the
local reason (the shared record keeps the outcome of the instance that wrote
it last). A plugin whose row changes is retried on the next pass regardless of
the backoff, so re-uploading a fixed build takes effect immediately.

**The reconciler never writes `plugins.status`.** The column is global while a
load failure is local — a bad disk, an unreachable object store, a half-written
file. An instance that wrote its own failure back would disable a working
plugin for the whole fleet, and it would be exactly the cross-instance feedback
channel this design does without. The loader and the supervisor still write
the outcome of the operations an operator (or a guest failure) triggered on
the instance they happened on; the reconciler tolerates such a row as the
table above describes.

## Missing plugin files

With `FILES_DRIVER=local` the plugin file only exists on the instance that
received the install. When the file is missing or does not match the recorded
checksum, the reconciler fetches it from the plugin store (store id from the
row's `source`) under the cluster-wide lock
`pluginsync:download:<store id>:<version>` (TTL 5m) and verifies it against
`plugins.checksum` — the value this deployment installed — rather than
against the store's own metadata, then writes it through the file manager.

Two cases are deliberately not recoverable:

- a plugin uploaded by hand (`source` starts with `file://`), because no other
  instance can obtain the file;
- a row with no checksum, from before the column existed, because writing
  unverifiable bytes into storage that other instances may share would spread
  the problem instead of containing it.

Both are logged with the error surfaced on the admin API. The fix is shared
plugin storage (`FILES_DRIVER=s3`) or reinstalling the plugin so the checksum
is recorded.

## Locking

The loader serialises every lifecycle operation on a plugin with a per-plugin
lock and takes the manager's lock inside it, so the reconciler and the admin
HTTP handlers never race on the same module. Handlers that span several steps
(download → save → load; unload → cleanup → delete) additionally `Hold` the
plugin so a pass that runs in between sees `ErrPluginHeld` instead of a
half-applied row.

**No host library may call into this package.** Guest `Initialize` and
`Shutdown` run under the manager's lock, so a host library reaching back here
would invert that order.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `PLUGIN_SYNC_DISABLED` | `false` | Turns the reconciler off entirely; `GET /loaded` then carries no `sync` object |
| `PLUGIN_SYNC_REFRESH_INTERVAL` | `60s` | Upper bound on staleness after a lost hint |
| `PLUGIN_SYNC_MIN_BACKOFF` | `15s` | First retry delay after a failed load |
| `PLUGIN_SYNC_MAX_BACKOFF` | `15m` | Retry delay ceiling |

Leaving it on with a single instance costs one indexed query per interval when
nothing has changed.
