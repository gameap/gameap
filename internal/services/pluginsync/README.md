# pluginsync

Keeps every panel instance's loaded plugins in step with the `plugins` table.

Installing, updating or removing a plugin changes the runtime of exactly one
instance — the one that handled the HTTP request. Without this package the other
replicas keep serving the old module (or none at all) until they are restarted,
and with them the plugin's HTTP routes, frontend bundle, server abilities and
RCON/Query protocol registrations.

## How it works

The database is the desired state. The reconciler compares it against what the
plugin manager currently has loaded and closes the gap.

A pub/sub message on `gameap:plugin:sync` carries **no state**. It only wakes
the reconciler, which then re-reads the database and decides for itself. That is
what makes the feature survive the transport it runs on: the pub/sub drivers are
at most once with no persistence and no replay, so a message can be lost,
duplicated or delivered out of order without changing the result. The worst a
lost message costs is one `PLUGIN_SYNC_REFRESH_INTERVAL` of staleness.

The same property makes the "who sent it" question irrelevant — an instance
handles its own hints too, because a pass is idempotent, and filtering on the
sender would break outright when `PUBSUB_INSTANCE_ID` is left unset and every
replica shares the same identifier.

Each pass:

1. reads every plugin row — **on a read failure it unloads nothing**, because an
   unreachable database must not drain the plugins of every instance that
   notices;
2. unloads what must not run: rows that went to `disabled`/`error`, and rows that
   disappeared;
3. reloads what changed, replacing the module in place so it is absent from the
   registry for a single map assignment rather than for a whole rebuild;
4. loads active rows that are not running here;
5. rebuilds the event subscriptions once, and only if something moved.

## What is not synced, and why

- **Permissions.** `allowed_permissions` is re-read from the database on every
  host library call (`internal/plugin/hostlibrary/pluginpermissions.go`), so
  granting or revoking one already takes effect everywhere without touching a
  runtime. Reloading on a permission change would be downtime that buys nothing,
  which is why it is not part of the reload fingerprint.
- **`config`.** It never reaches the guest: `internal/plugin/loader.go` passes a
  nil config to the manager, so `Initialize` receives nothing. There is
  currently no observable difference to synchronise. If the column is ever wired
  through, it belongs in the fingerprint, because `Initialize` is its only
  delivery point and delivering it does require a reload.
- **`priority`.** Nothing in the runtime reads it; it only orders repository
  queries.

The reload fingerprint is therefore `hash(version, filename, checksum)`.
`status` is handled separately, as a load or unload decision.

## Failure handling

A failed load is recorded in memory with an exponential backoff (15 s to 15 min
by default) and reported on `GET /api/admin/plugins/loaded`. A plugin whose row
changes is retried on the next pass regardless of the backoff, so re-uploading a
fixed build takes effect immediately.

**The reconciler never writes `plugins.status`.** The column is global while a
load failure is local — a bad disk, an unreachable object store, a half-written
file. An instance that wrote its own failure back would disable a working plugin
for the whole fleet, and it would be exactly the cross-instance feedback channel
this design does without. `plugininstall.TryLoadPlugin` still marks `error` on
the instance an operator is talking to, which is an operator-initiated,
synchronous decision; the reconciler tolerates such a row and loads it if it can.

## Missing plugin files

With `FILES_DRIVER=local` the plugin file only exists on the instance that
received the install. When the file is missing (or does not match the recorded
checksum) the reconciler fetches it from the plugin store under a cluster-wide
lock, and verifies it against `plugins.checksum` — the value this deployment
installed — rather than against the store's own metadata.

Two cases are deliberately not recoverable:

- a plugin uploaded by hand (`source` starts with `file://`), because no other
  instance can obtain the file;
- a row with no checksum, from before the column existed, because writing
  unverifiable bytes into storage that other instances may share would spread the
  problem instead of containing it.

Both are logged with the error surfaced on the admin API. The fix is shared
plugin storage (`FILES_DRIVER=s3`) or reinstalling the plugin so the checksum is
recorded.

## Locking

Lock order is always `Loader.applyMu` → `Manager.mu`. The loader is the single
writer of manager membership, so the reconciler and the admin HTTP handlers
serialise against each other there rather than through anything in this package.

**No host library may call into this package.** Guest `Initialize` and `Shutdown`
run under the manager's lock, so a host library reaching back here would invert
that order.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `PLUGIN_SYNC_DISABLED` | `false` | Turns the reconciler off entirely |
| `PLUGIN_SYNC_REFRESH_INTERVAL` | `60s` | Upper bound on staleness after a lost hint |
| `PLUGIN_SYNC_MIN_BACKOFF` | `15s` | First retry delay after a failed load |
| `PLUGIN_SYNC_MAX_BACKOFF` | `15m` | Retry delay ceiling |

Leaving it on with a single instance costs one indexed query per interval when
nothing has changed.
