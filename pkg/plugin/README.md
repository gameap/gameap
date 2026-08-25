# GameAP Plugin System

The GameAP plugin system allows extending functionality through WebAssembly-based plugins. Plugins run in a sandboxed environment for security while having access to GameAP's core functionality through host function libraries.

## Architecture

```
┌─────────────────────────────────────┐
│         GameAP Host                 │
├─────────────────────────────────────┤
│  Plugin Manager                     │
│  - Plugin Loading (.wasm)           │
│  - Lifecycle Management             │
│  - Event Dispatcher                 │
├─────────────────────────────────────┤
│  Host Function Libraries            │
│  - gameap-repository                │
│  - gameap-servercontrol             │
│  - gameap-cache                     │
│  - gameap-http                      │
│  - gameap-log                       │
└──────────────┬──────────────────────┘
               │ WASM Runtime (wazero)
               ▼
┌─────────────────────────────────────┐
│        Plugin (.wasm)               │
│  - Implements PluginService         │
│  - Calls Host Function Libraries    │
└─────────────────────────────────────┘
```

## Plugin Capabilities

Plugins can:
- **Hook into panel events** (server lifecycle and CRUD, daemon tasks, server
  settings, users, nodes and node presence, other plugins' lifecycle)
- **Access repositories** (servers, users, nodes, games, tasks, settings)
- **Control servers** (start, stop, restart, update, install)
- **Use caching** (get, set, delete)
- **Store credentials encrypted at rest** (gameap-secrets, under the `secrets` grant)
- **Introspect the panel** (gameap-host: own grants, host modules)
- **Make HTTP requests** (external API calls)
- **Log messages** (debug, info, warn, error)
- **Register custom HTTP endpoints** (extend the API)
- **Extend RCON/Query protocols** (add new games, override built-ins, implement new wire protocols)
- **Contribute translation & frontend files** (layered over the built-in filesystems, via `GetAssets`)

## Event Types

| Event | Trigger | Cancellable | Delivery |
|-------|---------|-------------|----------|
| `SERVER_PRE_START` | Before server start task created | Yes | Sync |
| `SERVER_POST_START` | After server start task created | No | Async |
| `SERVER_PRE_STOP` | Before server stop task created | Yes | Sync |
| `SERVER_POST_STOP` | After server stop task created | No | Async |
| `SERVER_PRE_RESTART` | Before server restart task created | Yes | Sync |
| `SERVER_POST_RESTART` | After server restart task created | No | Async |
| `SERVER_PRE_INSTALL` | Before server install task created | Yes | Sync |
| `SERVER_POST_INSTALL` | After server install task created | No | Async |
| `SERVER_PRE_UPDATE` | Before server update task created | Yes | Sync |
| `SERVER_POST_UPDATE` | After server update task created | No | Async |
| `SERVER_PRE_REINSTALL` | Before server reinstall workflow | Yes | Sync |
| `SERVER_POST_REINSTALL` | After server reinstall workflow | No | Async |
| `SERVER_PRE_DELETE` | Before server deletion | Yes | Sync |
| `SERVER_POST_DELETE` | After server deletion | No | Async |
| `SERVER_CREATED` | After server created in database | No | Async |
| `SERVER_UPDATED` | After server updated in database | No | Async |
| `SERVER_DELETED` | After server deleted/soft-deleted in database | No | Async |
| `SERVER_SETTINGS_CHANGED` | After a server's settings were saved through the API (`extra_data.changed_fields` names them) | No | Async |
| `DAEMON_TASK_CREATED` | After daemon task persisted for dispatch | No | Async |
| `DAEMON_TASK_STARTED` | When the daemon first reports the task as working | No | Async |
| `DAEMON_TASK_COMPLETED` | After daemon reported task success | No | Async |
| `DAEMON_TASK_FAILED` | After daemon reported task error/cancel or the task was abandoned | No | Async |
| `USER_CREATED` | After a user was created through the API | No | Async |
| `USER_UPDATED` | After a user was updated (admin API or own profile, `extra_data.source=profile`; `changed_fields` names the fields, never values) | No | Async |
| `USER_PRE_DELETE` | Before a user is deleted | Yes | Sync |
| `USER_DELETED` | After a user was deleted | No | Async |
| `NODE_CREATED` | After a node was enrolled (`extra_data.source=enrollment`, `daemon_version`) | No | Async |
| `NODE_UPDATED` | After a node was updated (`changed_fields` names the fields; keys and certificates by name only) | No | Async |
| `NODE_PRE_DELETE` | Before a node is deleted | Yes | Sync |
| `NODE_DELETED` | After a node was deleted | No | Async |
| `NODE_ONLINE` | A daemon session opened on this panel instance (`extra_data.daemon_version`, `instance_id`, `reconnect`) | No | Async |
| `NODE_OFFLINE` | A daemon session closed on this panel instance | No | Async |
| `PLUGIN_LOADED` | Another plugin was (re)loaded on this instance (`extra_data.trigger` = `install`, `reload`, `recovery`, `sync`) | No | Async |
| `PLUGIN_UNLOADED` | Another plugin was unloaded or uninstalled (`status` = `unloaded` / `uninstalled`) | No | Async |
| `PLUGIN_ERROR` | Another plugin failed to load or was disabled at runtime (`error` carries the reason, `extra_data.trigger` the load trigger or `runtime_disable`) | No | Async |

Payloads: `server_event` for `SERVER_*`, `task_event` for `DAEMON_TASK_*`,
`server_settings_event` (server id + the saved settings) for
`SERVER_SETTINGS_CHANGED`, `user_event` (`gameap.User`: id, login, email,
name, timestamps — never credentials) for `USER_*`, `node_event`
(`gameap.Node`, without keys or certificates) for `NODE_*` and `plugin_event`
(compact plugin id, name, version, status, error) for `PLUGIN_*`. Every
event carries `context.plugin_id` = the compact id of the receiving plugin;
`PLUGIN_*` events are never delivered to the plugin they are about.

Delivery semantics:

- **Sync** (cancellable pre-events) block the initiating request; a plugin returning
  `should_cancel` aborts the operation. Each plugin call is bounded by a per-call
  timeout (10s); on expiry the module is closed and the plugin is disabled until it
  is reloaded (see [Runtime limits and recovery](#runtime-limits-and-recovery) —
  the panel records the reason and reloads the plugin on its own). Calls into one
  plugin are serialized; a caller queued behind an in-flight call gives up at its
  own deadline with a "plugin is busy" error (the plugin stays enabled — its module
  was never touched).
- **Async** events are dispatched in a background goroutine (detached from the
  request, 60s total budget) — plugins cannot delay the caller, and delivery errors
  are only logged. Several async events emitted by one operation
  (e.g. `SERVER_POST_DELETE` → `SERVER_DELETED`) are delivered sequentially in
  that order. At most 64 background deliveries run concurrently; when the
  backlog is full (wedged plugins), further async events are dropped with an
  error log instead of blocking callers.
- Subscriptions are collected via `GetSubscribedEvents` at panel startup and
  refreshed automatically after runtime plugin install/update/uninstall.
- `DAEMON_TASK_CREATED` fires only for tasks that go through the gRPC task
  dispatcher (the default); legacy repository-only saves do not emit it.
- `NODE_ONLINE` / `NODE_OFFLINE` are raised by the panel instance the daemon
  is connected to (once per session, never from the cross-instance session
  broadcast), so a cluster sees each transition exactly once — on the
  instance that owns the session. `PLUGIN_*` events likewise describe the
  answering instance: a reload on instance A is a `PLUGIN_LOADED` on A only.

## Host Function Libraries

### gameap-log

Provides logging capabilities.

```go
logger := log.NewLogService()
logger.Log(ctx, &log.LogRequest{
    Level:   "info",
    Message: "Hello from plugin",
    Fields:  map[string]string{"key": "value"},
})
```

### gameap-cache

Provides caching capabilities. Keys are namespaced per plugin and values are
capped by `PLUGIN_CACHE_MAX_VALUE`; see [Storage and cache
quotas](#storage-and-cache-quotas) for what the cache does and does not
guarantee.

```go
cache := cache.NewCacheService()

// Set a value
cache.Set(ctx, &cache.CacheSetRequest{
    Key:        "my-key",
    Value:      []byte("my-value"),
    TtlSeconds: 3600,
})

// Get a value
resp, _ := cache.Get(ctx, &cache.CacheGetRequest{Key: "my-key"})
if resp.Found {
    value := resp.Value
}

// Delete a value
cache.Delete(ctx, &cache.CacheDeleteRequest{Key: "my-key"})
```

### gameap-http

Provides HTTP client capabilities for external API calls.

```go
http := http.NewHTTPService()

resp, _ := http.Fetch(ctx, &http.HTTPFetchRequest{
    Method:         "POST",
    Url:            "https://api.example.com/webhook",
    Headers:        map[string]string{"Content-Type": "application/json"},
    Body:           []byte(`{"message": "hello"}`),
    TimeoutSeconds: 30,
})

if resp.StatusCode == 200 {
    body := resp.Body
}
```

### gameap-crypto

Provides cryptographic capabilities for secure random generation and password hashing.

```go
crypto := crypto.NewCryptoService()

// Generate random uint64 in range [0, max)
randResp, _ := crypto.RandomUint64(ctx, &crypto.RandomUint64Request{
    Max: 1000,
})
randomNumber := randResp.Value

// Generate random string
strResp, _ := crypto.RandomString(ctx, &crypto.RandomStringRequest{
    Length:  32,
    Charset: proto.String("0123456789abcdef"), // optional, defaults to alphanumeric
})
randomString := strResp.Value

// Hash password with Argon2id (OWASP recommended defaults)
hashResp, _ := crypto.Argon2Hash(ctx, &crypto.Argon2HashRequest{
    Password: "mysecretpassword",
})
passwordHash := hashResp.Hash // PHC format: $argon2id$v=19$m=19456,t=2,p=1$salt$hash

// Hash with custom parameters
hashResp, _ = crypto.Argon2Hash(ctx, &crypto.Argon2HashRequest{
    Password: "mysecretpassword",
    Params: &crypto.Argon2Params{
        Memory:      32768, // KB
        Time:        3,     // iterations
        Parallelism: 2,
        SaltLength:  32,    // bytes
        KeyLength:   64,    // bytes
    },
})

// Verify password against hash
verifyResp, _ := crypto.Argon2Verify(ctx, &crypto.Argon2VerifyRequest{
    Password: "mysecretpassword",
    Hash:     passwordHash,
})
if verifyResp.Match {
    // Password is correct
}
```

### gameap-secrets

Stores the plugin's own credentials (API keys, bot tokens) encrypted at rest,
which the plaintext `gameap-storage` payloads are not suited for. Requires the
`secrets` grant; without it every method fails with the missing permission in
`error` — `Set`/`Delete` answer `success = false`, `Get` answers
`found = false` and `ListKeys` an empty list — so the module stays importable.

```go
secretsSvc := secrets.NewSecretsService()

// Store or replace a secret
setResp, _ := secretsSvc.Set(ctx, &secrets.SecretSetRequest{
    Key:   "steam_api_key",
    Value: "sk-live-0123456789",
})
if !setResp.Success {
    // setResp.Error names the reason: missing grant, invalid key, quota,
    // oversized value, or encryption not configured on the panel
}

// Read it back
getResp, _ := secretsSvc.Get(ctx, &secrets.SecretGetRequest{Key: "steam_api_key"})
if getResp.Found {
    apiKey := getResp.Value
}

// List the keys the plugin owns (never the values)
listResp, _ := secretsSvc.ListKeys(ctx, &secrets.SecretListKeysRequest{
    KeyPrefix: proto.String("steam_"), // optional
})

secretsSvc.Delete(ctx, &secrets.SecretDeleteRequest{Key: "steam_api_key"})
```

Rules the panel enforces:

- Values are encrypted with the panel's `ENCRYPTION_KEY` (AES-256-GCM) and the
  ciphertext is bound to the owning plugin and key, so a row copied elsewhere
  in the database no longer decrypts.
- With no `ENCRYPTION_KEY` configured a write is **refused** rather than stored
  in plaintext (`PLUGIN_SECRETS_REQUIRE_ENCRYPTION=false` opts out).
- Keys must match `^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`.
- Quotas: `PLUGIN_SECRETS_MAX_KEYS_PER_PLUGIN` (64 by default) and
  `PLUGIN_SECRETS_MAX_VALUE` (8 KiB by default).
- Secrets are private to the plugin that wrote them and survive a plugin
  reload or update; a `Get` from another plugin answers `found = false`.
  Uninstalling the plugin deletes its secrets together with its
  `gameap-storage` entries.

### gameap-host

Lets a plugin introspect itself and the panel it runs on. Open to every
plugin — no grant, no rate limit, no audit record. Every call is read-only.

```go
import "github.com/gameap/gameap/pkg/plugin/sdk/host"

hs := host.NewHostService()

// What the operator granted (read live from the database).
grants, _ := hs.GetGrants(ctx, &host.GetGrantsRequest{})
canRunCommands := slices.Contains(grants.Permissions, "node_commands")

// Panel version, plugin API version, host modules instantiated for this
// plugin and the id of the answering instance.
info, _ := hs.GetHostInfo(ctx, &host.GetHostInfoRequest{})
hasNodeFS := slices.Contains(info.Modules, "gameap-nodefs")
```

A transient load (the upload dry-run) has no database record and no index
entry, so `GetGrants` answers an empty list and `GetHostInfo.modules` is
empty; the panel and API versions are still filled in.

### gameap-scheduler

Lets a plugin register periodic tasks the panel invokes on schedule. Tasks
can be added and removed at any time (during `Initialize` or later), and
registrations are persisted in the panel database, so they survive panel
restarts and plugin reloads. `AddTask` is an upsert by task name — registering
the same name again replaces the definition, so re-registering on every load
is safe.

A plugin that uses this module **must** register a task handler before
calling `AddTask` (usually both in `init()`):

```go
func init() {
    proto.RegisterPluginService(&MyPlugin{})
    scheduler.RegisterScheduledTaskHandler(&myTaskHandler{})
}

type myTaskHandler struct{}

func (h *myTaskHandler) HandleScheduledTask(
    ctx context.Context,
    req *scheduler.HandleScheduledTaskRequest,
) (*scheduler.HandleScheduledTaskResponse, error) {
    // req.TaskName    — which task fired
    // req.ScheduledAt — unix milliseconds of the slot
    // req.Attempt     — 1-based, incremented on retries
    // Returning an error triggers the task's error policy.
    return &scheduler.HandleScheduledTaskResponse{}, nil
}
```

Registering and removing tasks:

```go
schedulerSvc := scheduler.NewSchedulerService()

resp, err := schedulerSvc.AddTask(ctx, &scheduler.AddTaskRequest{
    Name:       "stats-report",
    IntervalMs: 300_000, // every 5 minutes
    ErrorPolicy: &scheduler.ErrorPolicy{
        Policy:       scheduler.ErrorPolicyType_ERROR_POLICY_TYPE_RETRY,
        MaxRetries:   3,     // additional attempts after a failure
        RetryDelayMs: 1_000, // fixed delay between attempts...
        MaxJitterMs:  500,   // ...plus random 0..500ms
    },
    TimeoutMs: 5_000, // per-run handler budget; 0 = panel default
})
if err != nil {
    return err // the host call itself failed
}
if !resp.Success {
    // *resp.Error explains the rejection (bad interval, limits, ...)
}

schedulerSvc.RemoveTask(ctx, &scheduler.RemoveTaskRequest{Name: "stats-report"})
listResp, _ := schedulerSvc.ListTasks(ctx, &scheduler.ListTasksRequest{})
```

Scheduling semantics:

- Runs are aligned to Unix-epoch multiples of the interval, identically on
  every panel instance; in multi-instance deployments each slot is executed
  by exactly one instance (coordinated via Redis or database locks). NTP-level
  clock synchronization between instances is assumed.
- Missed slots are not backfilled: if the panel was down or a previous run
  was still in progress, the slot is skipped with a log entry.
- While a run (including its retries) is in progress, overlapping slots are
  skipped on all instances.
- With `ERROR_POLICY_TYPE_IGNORE` (default) a failed run is only logged; with
  `ERROR_POLICY_TYPE_RETRY` it is retried up to `MaxRetries` times with
  `RetryDelayMs` plus a random jitter up to `MaxJitterMs` between attempts.
- Interval floor, per-plugin task count, timeout ceilings and retry caps are
  panel-configured (`PLUGIN_SCHEDULER_*` environment variables).

The handler export is optional: plugins compiled without the scheduler module
keep loading and working unchanged.

### gameap-authz

Read-only permission checks. Available to every plugin — answering "may this
user do X" never changes state. Use it before acting on behalf of a user in
your own HTTP routes.

```go
az := authz.NewAuthzService()

// Global check: does the user hold ALL of these abilities?
resp, _ := az.Can(ctx, &authz.CanRequest{
    UserId:    userID,
    Abilities: []string{"admin roles & permissions"},
})
if resp.Error != nil {
    // The check could not be performed — do NOT read Allowed as a denial.
    return
}
if resp.Allowed {
    // ...
}

// At least one of them
az.CanOneOf(ctx, &authz.CanRequest{UserId: userID, Abilities: []string{"view", "edit"}})

// Scoped to one entity, e.g. a single game server
entityResp, _ := az.CanForEntity(ctx, &authz.CanForEntityRequest{
    UserId:     userID,
    EntityType: proto.EntityType_ENTITY_TYPE_SERVER,
    EntityId:   serverID,
    Abilities:  []string{"game-server-restart"},
})

// CanAnyForEntity works the same way but passes with a single ability.

rolesResp, _ := az.GetUserRoles(ctx, &authz.GetUserRolesRequest{UserId: userID})
// rolesResp.Roles -> ["admin"]
```

Abilities are plain strings: the panel's built-ins (`game-server-start`,
`view`, `edit`, …) as well as abilities contributed by plugins, which are
namespaced as `plugin:<plugin-id>:<name>`.

### gameap-rbac

Role and permission management: define your own roles with an individual set
of abilities instead of living with the panel's stock `admin` / `user`.

**Requires the `manage_rbac` permission.** Declare it in `GetInfo`:

```go
return &proto.PluginInfo{
    // ...
    RequiredPermissions: []string{"manage_rbac"},
}, nil
```

Without the grant the module stays importable, but every call answers with
`success = false` and an error naming the missing permission — check it and
degrade gracefully rather than assuming access.

```go
rb := rbac.NewRBACService()

// Create a role
saved, _ := rb.SaveRole(ctx, &rbac.SaveRoleRequest{
    Role: &rbac.Role{Name: "server-operator", Title: proto.String("Server Operator")},
})
if !saved.Success {
    // saved.Error explains why — missing permission, storage failure, ...
    return
}
roleID := saved.Role.Id

// Give the role an ability, scoped to one server
rb.Allow(ctx, &rbac.AbilitiesRequest{
    EntityType: proto.EntityType_ENTITY_TYPE_ROLE,
    EntityId:   roleID,
    Abilities: []*rbac.Ability{{
        Name:       "game-server-restart",
        EntityType: proto.EntityType_ENTITY_TYPE_SERVER.Enum(),   // *proto.EntityType
        EntityId:   proto.Uint64(serverID),
    }},
})

// Hand the role to a user
rb.AssignRolesForEntity(ctx, &rbac.AssignRolesRequest{
    EntityType: proto.EntityType_ENTITY_TYPE_USER,
    EntityId:   userID,
    Roles:      []*rbac.RestrictedRole{{Role: saved.Role}},
})

// Inspect what a role grants
perms, _ := rb.GetPermissions(ctx, &rbac.EntityRequest{
    EntityType: proto.EntityType_ENTITY_TYPE_ROLE,
    EntityId:   roleID,
})

// Replace a user's roles by name (every name must already exist)
rb.SetUserRoles(ctx, &rbac.SetUserRolesRequest{
    UserId:    userID,
    RoleNames: []string{"server-operator"},
})

// Grant or take away abilities from one user directly
rb.AllowUserAbilitiesForEntity(ctx, &rbac.UserAbilitiesRequest{
    UserId:     userID,
    EntityType: proto.EntityType_ENTITY_TYPE_SERVER,
    EntityId:   serverID,
    Abilities:  []string{"game-server-console-view"},
})

rb.DeleteRole(ctx, &rbac.DeleteRoleRequest{Id: roleID})
```

Notes:

- `AssignRolesForEntity` adds assignments; pair it with `ClearRolesForEntity`
  to replace them.
- `RevokeOrForbidUserAbilitiesForEntity` removes a user's direct grants and,
  when a role still grants the ability, records an explicit denial.
- Every mutation drops the panel's permission cache, so a following
  `gameap-authz` check sees the change immediately.

### gameap-servercontrol

Provides server control capabilities.

```go
sc := servercontrol.NewServerControlService()

// Start a server
resp, _ := sc.StartServer(ctx, &servercontrol.ServerControlRequest{
    ServerId: 123,
})
if resp.Success {
    taskID := resp.TaskId
}

// Stop, restart, update, install, reinstall work similarly
```

### gameap-nodefs

Provides file system operations on daemon nodes. Reading (`ReadDir`,
`Download`, `GetFileInfo`, `Hash`, `GetArchiveOperation`) requires the
`files_read` permission; everything that writes (`MkDir`, `Copy`, `Move`,
`Upload`, `Remove`, `Chmod`, archive create/extract) requires `files`, which
includes `files_read` (see [Plugin permissions](#plugin-permissions)). Paths
are also subject to the operator's [path policy](#path-policy).

```go
fs := nodefs.NewNodeFSService()

// Read directory contents
readDirResp, _ := fs.ReadDir(ctx, &nodefs.ReadDirRequest{
    NodeId: 1,
    Path:   "/home/servers",
})
for _, file := range readDirResp.Files {
    fmt.Printf("%s (%d bytes)\n", file.Name, file.Size)
}

// Download a file. Without Offset/Length this is the whole file in one
// message, so the panel caps it (PLUGIN_NODEFS_MAX_INLINE, 32 MiB by default)
// and a larger file answers with an error naming both sizes.
downloadResp, _ := fs.Download(ctx, &nodefs.DownloadRequest{
    NodeId: 1,
    Path:   "/home/servers/server.cfg",
})
content := downloadResp.Content

// Naming a window reads a larger file a piece at a time: only the window has
// to fit the cap. The answer echoes Offset and carries TotalSize, so paging
// needs no separate GetFileInfo. Reading at or past the end is not an error —
// it answers empty content, which is how the loop below ends. A Length above
// the cap is refused ("window too large: ..."), not quietly clamped; a full
// window is the ordinary answer and never an error.
var whole []byte
for offset := uint64(0); ; {
    page, _ := fs.Download(ctx, &nodefs.DownloadRequest{
        NodeId: 1,
        Path:   "/home/servers/world.tar.gz",
        Offset: offset,
        Length: 8 << 20,
    })
    if page.Error != nil || len(page.Content) == 0 {
        break
    }
    whole = append(whole, page.Content...)
    offset += uint64(len(page.Content))
    if offset >= page.TotalSize {
        break
    }
}

// Length: 0 with an Offset asks for "as much as the cap allows", so it is
// len(Content) — not the Length asked for — that says how much was served.
// Advancing by len(Content), as above, is therefore what a paging loop wants
// either way.

// To hand a whole large file to a *client* rather than read it here, answer
// the route with an HTTPResponse file reference instead: the panel streams it
// and the bytes never enter guest memory.

// Upload a file
fs.Upload(ctx, &nodefs.UploadRequest{
    NodeId:      1,
    Path:        "/home/servers/config.txt",
    Content:     []byte("server config"),
    Permissions: 0644,
})

// Create directory
fs.MkDir(ctx, &nodefs.MkDirRequest{
    NodeId: 1,
    Path:   "/home/servers/newdir",
})

// Copy/Move files
fs.Copy(ctx, &nodefs.CopyRequest{
    NodeId:      1,
    Source:      "/home/servers/file.txt",
    Destination: "/home/servers/file_backup.txt",
})

// Remove file or directory
fs.Remove(ctx, &nodefs.RemoveRequest{
    NodeId:    1,
    Path:      "/home/servers/oldfile.txt",
    Recursive: false,
})

// Get file info
infoResp, _ := fs.GetFileInfo(ctx, &nodefs.GetFileInfoRequest{
    NodeId: 1,
    Path:   "/home/servers/server.cfg",
})
if infoResp.Found {
    fmt.Printf("Size: %d, Modified: %d\n", infoResp.File.Size, infoResp.File.ModificationTime)
}

// Change permissions
fs.Chmod(ctx, &nodefs.ChmodRequest{
    NodeId:      1,
    Path:        "/home/servers/script.sh",
    Permissions: 0755,
})

// Compute checksums (blocking; directories yield per-file errors)
hashResp, _ := fs.Hash(ctx, &nodefs.HashRequest{
    NodeId:    1,
    Paths:     []string{"/home/servers/map.bsp"},
    Algorithm: nodefs.HashAlgorithm_HASH_ALGORITHM_SHA256,
})
for _, h := range hashResp.Results {
    fmt.Printf("%s: %s\n", h.Path, h.Hash)
}
```

#### Archive operations

Archives are created and extracted by the daemon; the node must announce the
`archive` capability (gameap-daemon with archive support). Two API shapes are
available:

All archive paths (`archive_path`, `base_path`, `sources`, `destination`)
are absolute node paths; every source must reside under `base_path` — entry
names inside the archive are stored relative to it, so a source outside the
base is rejected at start.

**Blocking** — `CreateArchive`/`ExtractArchive` wait for the final result.
`timeout_seconds` is the combined operation-and-wait budget; it is also
capped by the guest call deadline, so on a tight budget the call answers
`completed=false` with the `operation_id` and the operation keeps running:

```go
resp, _ := fs.CreateArchive(ctx, &nodefs.CreateArchiveRequest{
    NodeId:         1,
    ArchivePath:    "/home/servers/backup.tar.gz",
    BasePath:       "/home/servers",
    Sources:        []string{"/home/servers/maps"},
    TimeoutSeconds: 60,
})
switch {
case !resp.Success:
    // validation or start error in *resp.Error
case !resp.Completed:
    // still running; poll fs.GetArchiveOperation(resp.OperationId)
case resp.OpSuccess:
    // done, resp.ArchiveSize etc. describe the result
}
```

**Asynchronous** — `StartCreateArchive`/`StartExtractArchive` return the
`operation_id` immediately. With `report_progress: true` the panel pushes
progress into the optional `ArchiveEventsHandler` service; completion is
always pushed when the handler is registered, and remains observable via
`GetArchiveOperation` polling either way. `CancelArchive` requests
cancellation (the operation still finishes with an error starting with
`canceled`).

```go
// In init():
nodefs.RegisterArchiveEventsHandler(&myArchiveHandler{})

type myArchiveHandler struct {
    nodefs.EmptyArchiveEventsHandler // override only what you need
}

func (h *myArchiveHandler) HandleArchiveCompleted(
    _ context.Context, req *nodefs.HandleArchiveCompletedRequest,
) (*nodefs.HandleArchiveCompletedResponse, error) {
    // req.Success, req.ArchiveSize, ...
    return &nodefs.HandleArchiveCompletedResponse{}, nil
}

startResp, _ := fs.StartCreateArchive(ctx, &nodefs.CreateArchiveRequest{
    NodeId:         1,
    ArchivePath:    "/home/servers/backup.zip",
    BasePath:       "/home/servers",
    Sources:        []string{"/home/servers/maps"},
    ReportProgress: true,
})
```

Callbacks are delivered only on the panel instance where the operation was
started, and only for operations started by this plugin. Progress pushes are
coalesced: a slow handler sees fewer, fresher updates. Do not run a blocking
`CreateArchive` from inside a callback-heavy flow — while the guest is parked
in a host call, no callback can be delivered into it.

### gameap-nodecmd

Provides command execution on daemon nodes (`node_commands` permission).
`work_dir` is checked against the [path policy](#path-policy); an omitted
`work_dir` keeps the daemon's default.

```go
cmd := nodecmd.NewNodeCmdService()

// Execute a command
resp, _ := cmd.ExecuteCommand(ctx, &nodecmd.ExecuteCommandRequest{
    NodeId:  1,
    Command: "ls -la /home/servers",
    WorkDir: proto.String("/home"),
})

fmt.Printf("Exit code: %d\n", resp.ExitCode)
fmt.Printf("Output: %s\n", resp.Output)
```

### gameap-ssh

Opens SSH connections to hosts the plugin names and runs commands there. This
is the one thing `gameap-nodecmd` cannot do: it works through the daemon, and a
machine being provisioned does not have one yet.

Requires the `ssh` permission **and** `PLUGIN_SSH_ENABLED=true` on the panel.
The module is not registered at all when the operator leaves the switch off, so
a plugin that imports it fails to load — check the panel configuration first
when a load fails with an unknown import.

```go
sshc := ssh.NewSSHService()

// 1. A key for the machine that does not exist yet.
key, _ := sshc.GenerateKeyPair(ctx, &ssh.GenerateKeyPairRequest{
    Type:    ssh.KeyType_KEY_TYPE_ED25519,
    Comment: "autoscale@panel",
})
// key.PublicKey goes to the cloud provider; keep key.PrivateKeyPem yourself —
// in gameap-secrets, which encrypts at rest, not in gameap-storage, whose
// payloads are stored in plaintext.

// 2. First contact: no host key is known yet, so accept what answers and pin
//    the fingerprint the response reports for every later connection.
conn, _ := sshc.Connect(ctx, &ssh.ConnectRequest{
    Host:    "203.0.113.10",
    User:    "root",
    Auth:    &ssh.SSHAuth{PrivateKeyPem: key.PrivateKeyPem},
    HostKey: &ssh.HostKeyPolicy{AcceptAny: true},
})
if !conn.Success {
    return
}
// store conn.HostKeyFingerprintSha256, then reconnect with
// &ssh.HostKeyPolicy{FingerprintsSha256: []string{stored}}

// 3. A short command answers inline, and the connection is done with.
out, _ := sshc.Exec(ctx, &ssh.ExecRequest{Handle: conn.Handle, Command: "uname -a"})
if out.Completed && out.OpSuccess {
    log.Info(string(out.Stdout))
}

sshc.Disconnect(ctx, &ssh.DisconnectRequest{Handle: conn.Handle})
```

A daemon install takes minutes, so it runs asynchronously. `Disconnect` cancels
every operation still running on the connection, so the handle has to stay open
until the completion callback arrives:

```go
op, _ := sshc.StartExec(ctx, &ssh.ExecRequest{
    Handle:  conn.Handle,
    Command: "bash -s",
    Stdin:   []byte(installScript), // from gameap-nodes.CreateSetupKey
})
// Keep conn.Handle open here; persist it together with op.OperationId.
```

Completion is pushed into the plugin when it exports the handler:

```go
func init() {
    ssh.RegisterSSHExecEventsHandler(&myPlugin{})
}

func (p *myPlugin) HandleExecCompleted(
    ctx context.Context, req *ssh.HandleExecCompletedRequest,
) (*ssh.HandleExecCompletedResponse, error) {
    // Fetch the output with GetExecOperation; persist what you need, because
    // plugin memory does not survive a reload.

    // The install has finished, so the connection can go now.
    sshc.Disconnect(ctx, &ssh.DisconnectRequest{Handle: storedHandle})

    return &ssh.HandleExecCompletedResponse{}, nil
}
```

Semantics worth knowing:

- **`Exec` blocks, `StartExec` does not.** `Exec` waits inside the guest call
  deadline; when the budget runs out it answers `completed=false` with an
  operation id and subscribes to the completion callback. A command that
  finishes in time triggers no callback, so short commands stay quiet.
- **Handles and operations are instance-local.** They live in the memory of the
  panel instance that created them and do not survive a plugin reload. On a
  multi-instance panel a later scheduler tick may run elsewhere, where
  `GetExecOperation` answers `found=false` — prefer the completion callback
  (delivered on the owning instance) and persist the result.
- **A host key policy is mandatory.** Either `accept_any` or a pin list —
  never both: the combination is rejected, because `accept_any` would silently
  skip the pins. The observed key is always returned so first contact can pin
  it, and an operator can forbid `accept_any` panel-wide with
  `PLUGIN_SSH_ALLOW_ACCEPT_ANY_HOST_KEY=false`.
- **Environment variables** go through SSH `env` requests, which most sshd
  configurations accept only for the names in `AcceptEnv`. Use
  `env K=V command` inside the command for anything else.
- **Output is capped** per stream (`PLUGIN_SSH_MAX_OUTPUT_BYTES`); the head is
  kept and the stream is flagged truncated, with the produced byte count
  reported separately.
- **File helpers** (`WriteFile`, `ReadFile`) stream through a remote `cat`, so
  no SFTP subsystem is needed on a freshly installed machine. `WriteFile`
  writes to a same-directory temporary file under `umask 077` and renames it
  into place: the content is never readable wider than the requested mode, and
  a failed transfer leaves no partial target. `mode` is octal permission bits
  up to `0o777` — write `0o644`, not decimal `644`; larger values are rejected.

Targets go through the same address policy as `gameap-http`: cloud-metadata
addresses are never reachable, and private addresses are refused unless the
operator allows them (`PLUGIN_SSH_BLOCK_PRIVATE_IPS`, `PLUGIN_SSH_ALLOWED_HOSTS`).

### Repository Services

Repository access is split into separate services for better organization:

#### gameap-servers

```go
serversRepo := servers.NewServersService()

// Find servers
serversResp, _ := serversRepo.FindServers(ctx, &servers.FindServersRequest{
    Filter: &servers.ServerFilter{
        Enabled: proto.Bool(true),
    },
    Pagination: &common.Pagination{
        Limit:  10,
        Offset: 0,
    },
})

// Get a single server
serverResp, _ := serversRepo.GetServer(ctx, &servers.GetServerRequest{Id: 123})
if serverResp.Found {
    server := serverResp.Server
}
```

#### gameap-games

```go
gamesRepo := games.NewGamesService()

gameResp, _ := gamesRepo.GetGame(ctx, &games.GetGameRequest{Code: "cs"})
if gameResp.Found {
    game := gameResp.Game
}
```

#### gameap-gamemods

```go
gameModsRepo := gamemods.NewGameModsService()

gameModResp, _ := gameModsRepo.GetGameMod(ctx, &gamemods.GetGameModRequest{Id: 1})
if gameModResp.Found {
    gameMod := gameModResp.GameMod
}
```

#### gameap-users

```go
usersRepo := users.NewUsersService()

userResp, _ := usersRepo.GetUser(ctx, &users.GetUserRequest{Id: 1})
if userResp.Found {
    user := userResp.User
}
```

#### gameap-nodes

Reading is open to every plugin:

```go
nodesRepo := nodes.NewNodesService()

nodeResp, _ := nodesRepo.GetNode(ctx, &nodes.GetNodeRequest{Id: 1})
if nodeResp.Found {
    node := nodeResp.Node
    // node.Metadata is a free-form string map — the panel never puts secrets
    // there, and neither should a plugin.
}
```

Writing and the enrollment setup keys require the `manage_nodes` permission.

```go
// Relabel a node. Metadata is merged, so keys other actors own survive;
// remove_metadata_keys deletes the ones you name.
nodesRepo.UpdateNode(ctx, &nodes.UpdateNodeRequest{
    Id:       1,
    Location: proto.String("fsn1"),
    Metadata: map[string]string{"hetzner.server_id": "42"},
})

// Retire it. This is a soft delete and is refused while game servers are
// still assigned to the node.
nodesRepo.DeleteNode(ctx, &nodes.DeleteNodeRequest{Id: 1})
```

Provisioning a new node — the panel never creates one directly, a daemon
enrolls itself with a single-use key:

```go
key, _ := nodesRepo.CreateSetupKey(ctx, &nodes.CreateSetupKeyRequest{
    // Applied to the node the panel creates when the daemon enrolls; this is
    // how you recognise your machine among the nodes that appear.
    Presets: &nodes.NodePresets{
        Name:     proto.String("hz-fsn1-7"),
        Location: proto.String("fsn1"),
        Provider: proto.String("Hetzner"),
        Metadata: map[string]string{"hetzner.server_id": "42"},
    },
    TtlSeconds: 3600,
})

// key.InstallScript is the same installer the admin setup link serves; pipe it
// into a root shell on the machine (gameap-ssh does exactly this).
// key.SetupKey is returned once — the panel stores only its digest.

status, _ := nodesRepo.GetSetupKey(ctx, &nodes.GetSetupKeyRequest{TicketId: key.TicketId})
if status.Found && status.Status == nodes.SetupKeyStatus_SETUP_KEY_STATUS_ENROLLED {
    nodeID := status.NodeId // the node the daemon became
}
```

The panel must know the address daemons dial: set `GRPC_EXTERNAL_HOST`, or pass
`connect_host` in the request. Without either, `CreateSetupKey` fails instead of
emitting a script the machine cannot use. Keys belong to the plugin that issued
them; another plugin's key answers `found=false`.

#### gameap-daemontasks

```go
tasksRepo := daemontasks.NewDaemonTasksService()

resp, _ := tasksRepo.CreateDaemonTask(ctx, &daemontasks.CreateDaemonTaskRequest{
    NodeId:   1,
    ServerId: proto.Uint64(123),
    TaskType: "gsstart",
})
```

#### gameap-serversettings

```go
settingsRepo := serversettings.NewServerSettingsService()

// Save server setting
settingsRepo.SaveServerSetting(ctx, &serversettings.SaveServerSettingRequest{
    ServerId: 123,
    Name:     "custom_setting",
    Value:    "value",
})
```

## Plugin Development

### Requirements

- Go 1.21+ or TinyGo 0.30+ (for Go plugins)
- protoc with protoc-gen-go-plugin

### Plugin Structure

```go
//go:build wasip1

package main

import (
    "context"

    "github.com/gameap/gameap/pkg/plugin/proto"
    "github.com/gameap/gameap/pkg/plugin/sdk/log"
)

func main() {}

func init() {
    proto.RegisterPluginService(MyPlugin{})
}

type MyPlugin struct{}

func (p MyPlugin) GetInfo(ctx context.Context, req *proto.GetInfoRequest) (*proto.PluginInfo, error) {
    return &proto.PluginInfo{
        Id:          "my-plugin",
        Name:        "My Plugin",
        Version:     "1.0.0",
        Description: "Example plugin",
        Author:      "Your Name",
        ApiVersion:  "1",
    }, nil
}

func (p MyPlugin) Initialize(ctx context.Context, req *proto.InitializeRequest) (*proto.InitializeResponse, error) {
    // Read configuration from req.Config
    return &proto.InitializeResponse{
        Result: &proto.Result{Success: true},
    }, nil
}

func (p MyPlugin) Shutdown(ctx context.Context, req *proto.ShutdownRequest) (*proto.ShutdownResponse, error) {
    return &proto.ShutdownResponse{
        Result: &proto.Result{Success: true},
    }, nil
}

func (p MyPlugin) GetSubscribedEvents(ctx context.Context, req *proto.GetSubscribedEventsRequest) (*proto.GetSubscribedEventsResponse, error) {
    return &proto.GetSubscribedEventsResponse{
        Events: []proto.EventType{
            proto.EventType_EVENT_TYPE_SERVER_POST_START,
            proto.EventType_EVENT_TYPE_SERVER_POST_STOP,
        },
    }, nil
}

func (p MyPlugin) HandleEvent(ctx context.Context, event *proto.Event) (*proto.EventResult, error) {
    logger := log.NewLogService()

    switch event.Type {
    case proto.EventType_EVENT_TYPE_SERVER_POST_START:
        serverEvent := event.GetServerEvent()
        logger.Log(ctx, &log.LogRequest{
            Level:   "info",
            Message: "Server started: " + serverEvent.Server.Name,
        })
    }

    return &proto.EventResult{Handled: true}, nil
}

func (p MyPlugin) GetHTTPRoutes(ctx context.Context, req *proto.GetHTTPRoutesRequest) (*proto.GetHTTPRoutesResponse, error) {
    return &proto.GetHTTPRoutesResponse{Routes: nil}, nil
}

func (p MyPlugin) HandleHTTPRequest(ctx context.Context, req *proto.HTTPRequest) (*proto.HTTPResponse, error) {
    return &proto.HTTPResponse{StatusCode: 404}, nil
}
```

### Building Plugins

Plugins must be built as WASM reactor modules using `-buildmode=c-shared`. This creates a module with `_initialize` entry point instead of `_start`, which is required for proper function exports.

**Using TinyGo (recommended, smaller binary size):**

```bash
tinygo build -o my-plugin.wasm -target=wasip1 -buildmode=c-shared .
```

**Using standard Go:**

```bash
GOOS=wasip1 GOARCH=wasm go build -o my-plugin.wasm -buildmode=c-shared .
```

**Important:** The `-buildmode=c-shared` flag is required. Without it, the WASM module will be a "command" module that exits after `main()`, and exported functions won't work properly.

### Custom HTTP Endpoints

Plugins can register custom HTTP endpoints:

```go
func (p MyPlugin) GetHTTPRoutes(ctx context.Context, req *proto.GetHTTPRoutesRequest) (*proto.GetHTTPRoutesResponse, error) {
    return &proto.GetHTTPRoutesResponse{
        Routes: []*proto.HTTPRoute{
            {
                Path:         "/my-plugin/status",
                Methods:      []string{"GET"},
                RequiresAuth: true,
                AdminOnly:    false,
                Description:  "Get plugin status",
            },
            {
                Path:         "/my-plugin/config",
                Methods:      []string{"GET", "POST"},
                RequiresAuth: true,
                AdminOnly:    true,
                Description:  "Plugin configuration",
            },
        },
    }, nil
}

func (p MyPlugin) HandleHTTPRequest(ctx context.Context, req *proto.HTTPRequest) (*proto.HTTPResponse, error) {
    switch req.Path {
    case "/my-plugin/status":
        return &proto.HTTPResponse{
            StatusCode: 200,
            Headers:    map[string]string{"Content-Type": "application/json"},
            Body:       []byte(`{"status": "ok"}`),
        }, nil
    }
    return &proto.HTTPResponse{StatusCode: 404}, nil
}
```

#### Serving node files

A route can hand the client a file that lives on a node without the bytes
passing through the plugin: answer with `File` instead of `Body` and the
panel streams it from the daemon itself, so neither the guest memory nor the
guest call deadline bounds the size — the transfer runs after the call returns
and does not hold the plugin's call gate. (The 1 MB cap is on the *request*
body, `DefaultMaxBodySize`; a plugin's own response body is bounded only by
guest memory.)

```go
case "/my-plugin/backups/latest":
    return &proto.HTTPResponse{
        StatusCode: 200,
        Headers:    map[string]string{"Content-Type": "application/gzip"},
        File: &proto.FileRef{
            NodeId:   1,
            Path:     "/home/servers/cs2/backups/latest.tar.gz",
            Filename: "cs2-latest.tar.gz", // Content-Disposition name; empty → base name of Path
        },
    }, nil
```

Rules:

- Requires the `files_read` grant, which `files` includes — the same grant that
  gates reading through `gameap-nodefs`, so a plugin can only stream what it
  could read itself. Without it the panel answers `403` and records an
  `access.denied` audit event. The check happens on every request, at the moment
  the file is served, and is subject to `PLUGIN_PERMISSIONS_ENFORCE` like every
  other grant check.
- Only authenticated clients receive files: on a route with `RequiresAuth:
  false` an anonymous request gets `401`, whatever the plugin answered.
- `Body` is ignored when `File` is set. `StatusCode` (default `200`) is the
  plugin's. Of the plugin's headers only `Content-Type`, `Content-Language`,
  `Cache-Control`, `Expires`, `Pragma`, `Last-Modified`, `ETag`, `Vary` and
  the plugin metadata headers `X-Plugin` / `X-Plugin-*` reach the client —
  everything else (`Set-Cookie`, `Location`, `WWW-Authenticate`, CSP, other
  `X-*` names such as `X-Accel-Redirect`, …) is dropped, as the response is
  served from the panel origin. The panel owns `Content-Length` and
  `Content-Disposition` (always an attachment). The declared length is the one
  the stat reported and the copy is bounded by it, so a file that changed
  between the stat and the read ends the response as an error rather than as a
  body that quietly disagrees with its header.
- Range requests are served (`Accept-Ranges: bytes`), so an interrupted download
  resumes instead of starting over: a satisfiable `Range` answers `206` with
  `Content-Range`, and one past the end answers `416`. A single range only —
  a multi-range header is ignored and the whole file is sent, which is what a
  resuming client wants anyway. A range is not applied when the plugin named its
  own status code, since that answer is the plugin's to define.
- `..` path segments are rejected, and the path is subject to the operator's
  path policy.
- Panels that predate this field ignore it and send an empty body, so a plugin
  that must run on older panels should keep a `Body` fallback for them.

### Cancelling Events

For `PRE_*` events (`SERVER_PRE_*`, `USER_PRE_DELETE`, `NODE_PRE_DELETE`),
plugins can prevent the operation by setting `ShouldCancel`; the initiating
request is answered with `409 Conflict` and the plugin's message:

```go
func (p MyPlugin) HandleEvent(ctx context.Context, event *proto.Event) (*proto.EventResult, error) {
    if event.Type == proto.EventType_EVENT_TYPE_SERVER_PRE_START {
        serverEvent := event.GetServerEvent()

        // Check some condition
        if shouldPreventStart(serverEvent.Server) {
            return &proto.EventResult{
                Handled:      true,
                ShouldCancel: true,
                Message:      proto.String("Server start blocked by plugin"),
            }, nil
        }
    }

    return &proto.EventResult{Handled: true}, nil
}
```

### Contributing translation & frontend files

Implement `GetAssets` to ship translation files (served at `/lang/`) and frontend static
files (served under the SPA root). Each group is layered **over** the built-in filesystem,
so a plugin file shadows a core file of the same path and a new file is simply added — the
first match in the first layer wins. Files are resolved per request, so a plugin installed
at runtime contributes immediately.

```go
//go:embed assets/i18n/es.json
var i18nES []byte

//go:embed assets/frontend/plugins/my-plugin/meta.json
var frontendMeta []byte

func (p *MyPlugin) GetAssets(
    _ context.Context,
    _ *proto.GetAssetsRequest,
) (*proto.GetAssetsResponse, error) {
    return &proto.GetAssetsResponse{
        I18NFiles: []*proto.AssetFile{
            {Path: "es.json", Content: i18nES}, // -> /lang/es.json
        },
        FrontendFiles: []*proto.AssetFile{
            {Path: "plugins/my-plugin/meta.json", Content: frontendMeta}, // -> /plugins/my-plugin/meta.json
        },
    }, nil
}
```

Guidelines:

- **Paths** must be valid, unrooted (no leading `/`), and free of `..`. Invalid paths are skipped.
- **Additive by convention.** Namespace frontend files (e.g. under `plugins/<id>/`). Do **not**
  ship `index.html`: the Content-Security-Policy is hashed from the built-in `index.html`, so an
  overriding `index.html` would have its inline scripts blocked.
- **Whole-file shadowing.** A plugin `en.json` replaces the core `en.json` entirely (it is not a
  key-level JSON merge). Prefer adding new locale files (`es.json`, `de.json`, …) with a top-level
  `_language` label (`{"name": "English name", "native_name": "own name"}`) so the locale appears in the
  UI language switcher (served by `GET /lang`).
- **Limits.** Each file is capped at 8 MiB and each group at 64 MiB; oversized files are skipped.

## RCON / Query Protocol Extension

Plugins can add support for new games, override the built-in game→protocol
mappings, and implement entirely new RCON/Query wire protocols. The panel
consults plugin registrations **before** its built-in tables, so a plugin
registration overrides a built-in for the same game/engine.

Protocol extension is a **separate, optional service** — `ProtocolService` in
`pkg/plugin/sdk/protocol` — so the core `PluginService` stays lean. A plugin
opts in by implementing `ProtocolService` (embed `protocol.EmptyProtocolService`
for defaults) and registering it alongside the core service:

```go
import (
    "github.com/gameap/gameap/pkg/plugin/proto"
    "github.com/gameap/gameap/pkg/plugin/sdk"
    "github.com/gameap/gameap/pkg/plugin/sdk/protocol"
)

type MyPlugin struct {
    sdk.EmptyPluginService       // core lifecycle/events/http
    protocol.EmptyProtocolService // optional RCON/Query extension
}

func init() {
    p := &MyPlugin{}
    proto.RegisterPluginService(p)
    protocol.RegisterProtocolService(p)
}
```

### Registering protocols

Implement `GetRconProtocols` / `GetQueryProtocols`. Each registration lists the
`game_codes` and `engines` it applies to and picks a transport:

- `RCON_TRANSPORT_BUILTIN` and `QUERY_TRANSPORT_BUILTIN`, both with a
  `builtin_protocol` name — reuse the panel's engine. Pure mapping: no plugin
  code runs at execute time, the one exception being `parse_via_plugin` below.
  Use this to add a new game that speaks a protocol the panel already
  implements.
  - RCON names: `source`, `goldsource`, `quake2`, `quake3`, `samp`, `battleye`.
  - Query names: `source`, `minecraft`, `gamespy2`, `gamespy3`, `quake2`,
    `quake3`, `samp`, `raknet`.
  - An RCON registration naming a protocol the panel does not implement is
    **dropped with a warning**, so a typo cannot shadow the built-in tables and
    leave the game without RCON.
  - `RCON_TRANSPORT_BUILTIN_SOURCE` and `RCON_TRANSPORT_BUILTIN_GOLDSOURCE` are
    older shorthands for `RCON_TRANSPORT_BUILTIN` with `builtin_protocol`
    `"source"` / `"goldsource"`. They keep working; new plugins should not use
    them, because a protocol the panel adds later needs no new enum value.
- `RCON_TRANSPORT_PLUGIN` / `QUERY_TRANSPORT_PLUGIN` — the plugin implements the
  wire protocol (see below). RCON connections are always TCP here; a UDP-based
  RCON protocol has to be a built-in one.

```go
func (p MyPlugin) GetRconProtocols(ctx context.Context, req *protocol.GetRconProtocolsRequest) (*protocol.GetRconProtocolsResponse, error) {
    return &protocol.GetRconProtocolsResponse{Protocols: []*protocol.RconProtocol{{
        Id:        "mygame-rcon",
        GameCodes: []string{"mygame"},
        Transport:       protocol.RconTransport_RCON_TRANSPORT_BUILTIN,
        BuiltinProtocol: "source",
        Players: &protocol.PlayerCapability{
            Supported:      true,
            PlayersCommand: "status",
            KickCommand:    "kickid {id} {reason}",  // templates: {id} {name} {uniqid} {reason} {duration}
            BanCommand:     "banid {duration} {id}",
            ParseViaPlugin: true,                     // route players output to ParsePlayers
        },
    }}}, nil
}
```

Player management: kick/ban are command templates the host renders (`{duration}`
is whole seconds). The players-list output is parsed by the built-in parser, or
by your `ParsePlayers` RPC when `parse_via_plugin` is set.

`parse_via_plugin` is the one case where a built-in transport calls back into
the plugin, so a registration that sets it **must** implement `ParsePlayers`
(see `examples/protocol-extension/main.go`). Leaving it on the embedded
`protocol.EmptyProtocolService` default makes every players-list request fail
with "not implemented"; there is no fallback to the built-in parser. Everything
else keeps working — commands still execute over the built-in engine, and
kick/ban still render from their templates.

Leave `kick_command` or `ban_command` empty when your game has no such command:
the panel then hides that button instead of offering one that fails. The
features endpoint reports `playersList`, `playersKick` and `playersBan`
separately for exactly this reason.

### Implementing a wire protocol (gameap-net)

For `*_TRANSPORT_PLUGIN`, the host opens and guards the TCP/UDP connection to the
game server (address from the server record, timeouts, IP policy) and hands the
plugin a `conn_handle`. The plugin performs I/O over that handle with the
**gameap-net** host library — it never dials, so it can only ever reach the
server the host opened for it.

- RCON: implement `RconOpen` (authenticate over the handle), `RconExecute` (run
  one command), and optionally `RconClose`.
- Query: implement `QueryServer` (returns a `QueryResult`).

```go
func (p MyPlugin) QueryServer(ctx context.Context, req *protocol.QueryServerRequest) (*protocol.QueryServerResponse, error) {
    n := net.NewNetService() // github.com/gameap/gameap/pkg/plugin/sdk/net
    probe := []byte("\xFF\xFF\xFF\xFFping")

    sent, err := n.Send(ctx, &net.NetSendRequest{Handle: req.ConnHandle, Data: probe})
    if err != nil {
        return queryError(err.Error()), nil
    }
    if sent.Error != nil {
        return queryError(*sent.Error), nil
    }

    resp, err := n.Recv(ctx, &net.NetRecvRequest{Handle: req.ConnHandle, MaxBytes: 1400, TimeoutMs: 1000})
    if err != nil {
        return queryError(err.Error()), nil
    }
    if resp.Error != nil {
        return queryError(*resp.Error), nil
    }

    // parse resp.Data ...
    return &protocol.QueryServerResponse{Result: &protocol.QueryResult{Online: len(resp.Data) > 0}}, nil
}

func queryError(msg string) *protocol.QueryServerResponse {
    return &protocol.QueryServerResponse{Error: &msg}
}
```

The gameap-net library is gated by `PLUGIN_NET_*` config (enable, per-read size
and timeout caps, per-plugin connection cap, and a private-IP dial policy —
cloud-metadata IPs are always blocked). See
`pkg/plugin/examples/protocol-extension/` for a complete example.

## Security

- Plugins run in a WebAssembly sandbox
- No direct filesystem access
- No direct network access by default — use gameap-http for external calls, or
  gameap-net for protocol I/O on host-opened connections (plugins never dial).
  The single exception is gameap-ssh, where a plugin does name its own target
  because bootstrapping a machine cannot work otherwise; it is gated on the
  `ssh` grant and on the operator's `PLUGIN_SSH_ENABLED` switch, and its
  targets go through the same address policy as gameap-http
- Node metadata is readable by every plugin through gameap-nodes, so it is not
  a place for secrets
- Plugin configuration can restrict capabilities

### Plugin permissions

Privileged host modules are gated on the plugin's own grants, kept in the
`allowed_permissions` column of its database record:

| Permission | Gates |
|---|---|
| `manage_servers` | `gameap-servercontrol` (every operation), `gameap-daemontasks.CreateDaemonTask`, `gameap-servers.SaveServer` / `DeleteServer`, `gameap-serversettings.SaveServerSetting` |
| `node_commands` | `gameap-nodecmd.ExecuteCommand` and `cmdexec` daemon tasks (on top of `manage_servers`) — an arbitrary shell command on a node |
| `files_read` | `gameap-nodefs` reads — `ReadDir`, `Download`, `GetFileInfo`, `Hash`, `GetArchiveOperation` — and `HTTPResponse.file`, a route answering with a node file |
| `files` | every `gameap-nodefs` operation, including writes, `Chmod` and archive create/extract; includes `files_read` |
| `listen_events` | Event subscriptions: a plugin without it is never called for events (its `GetSubscribedEvents` answer is ignored, with a warning in the log) |
| `manage_rbac` | `gameap-rbac` — creating roles, granting and revoking abilities |
| `secrets` | `gameap-secrets` — reading, writing, listing and deleting the plugin's encrypted credentials |
| `ssh` | every `gameap-ssh` operation — connecting to a host the plugin names itself, running commands on it and transferring files, all outside the daemon and the node inventory; key generation included, so a plugin cannot mint credentials without the grant. The grant alone is not enough: the operator must also set `PLUGIN_SSH_ENABLED=true` |
| `manage_nodes` | `gameap-nodes` — the mutating calls: `UpdateNode`, `DeleteNode` and the enrollment setup keys. Reads stay open to every plugin. The module checks this grant itself rather than through the guard, so it is enforced whatever `PLUGIN_PERMISSIONS_ENFORCE` says and carries no rate limit of its own |

`manage_games`, `manage_game_mods` and `manage_users` are reserved for write
operations the repository modules do not expose yet.
Reads — `gameap-servers.Find/Get`, `gameap-users`, `gameap-nodes`,
`gameap-games`, `gameap-gamemods`, `gameap-daemontasks.Find`,
`gameap-serversettings.Find`, `gameap-authz` — and `gameap-http`,
`gameap-cache`, `gameap-storage`, `gameap-crypto`, `gameap-log`,
`gameap-scheduler`, `gameap-net`, `gameap-host` are available to every
plugin.

A grant may be implied by a wider one (`files` ⊇ `files_read`): the guard,
the dry-run report and the admin UI all honour the relation, so a plugin that
declares `files_read` is satisfied by an existing `files` grant, and
`missing_permissions` never lists a permission whose superset is granted.

The policy table lives in `internal/plugin/hostlibrary/policy.go`; a test
checks it against the exported functions of the generated SDK glue, so a new
host function cannot slip in ungated.

A plugin declares what it needs in `PluginInfo.RequiredPermissions`, and
**installing grants exactly the declared permissions** (upload, store and
`PLUGINS_AUTOLOAD` alike). Unknown permission names are dropped rather than
stored. Installations that predate a gate were grandfathered the grant by a
migration (`files` in 015; `manage_servers`, `node_commands` and
`listen_events` in 020), so an upgrade never takes a working plugin's access
away.

The panel also reads the module's import section: a guest can only call what
it imports, so the dry-run endpoint (`POST /api/admin/plugins/upload/dry-run`)
reports `used_permissions` next to `required_permissions`, and
`undeclared_permissions` — what the plugin uses but does not declare, i.e.
what the install will refuse. `GET /api/admin/plugins/loaded` reports
`required_permissions`, `allowed_permissions`, `used_permissions` and
`missing_permissions` per plugin, and the admin UI shows the same in the
plugin's permissions dialog, highlighting the row's Permissions action when
a grant is missing.

Grants are operator-managed: `PUT /api/admin/plugins/{id}/permissions` with
`{"allowed_permissions": [...]}` replaces them (the admin UI offers checkboxes
in the permissions dialog, opened from the plugin row's Permissions action).
Grants are cached in each instance's memory: they are consulted on every
privileged host call and on every event delivery, and they only change when an
operator edits them. The endpoint announces every change over pub/sub
(`gameap:plugin:subscriptions:refresh`), and each instance drops its cached
grants for that plugin before rebuilding its subscription map — so a change
takes effect at once on the instance that made it, and on the others as soon
as the message arrives. `PLUGIN_PERMISSIONS_CACHE_TTL` (default `30s`) is the
backstop if the broker is unreachable; `0` disables the cache and reads the
record on every check. A grant set that is empty is never cached, so a plugin
that has just been installed is never denied by a stale answer.

Enforcement is transitional: `PLUGIN_PERMISSIONS_ENFORCE` (default `false` in
this release, `true` in a future one) decides whether the grants are applied.
While it is off every check passes — host calls, event deliveries and file
refs work without grants — but the grants are still recorded, computed and
editable, the load log names what is missing, and the admin permissions
dialog warns that they are not applied. Record the grants your plugin needs
now, so nothing breaks when enforcement becomes the default.

`listen_events` is checked twice: the subscription map each instance builds is
filtered by the grant, and every delivery re-checks it, so a revocation stops
events on the other instances as soon as the announcement lands rather than at
their next refresh. Uninstalling drops the cache on the instance that handled
it; the other instances keep the module loaded until they restart in any case.
Updating a plugin does not widen its grants: a
version that starts using `gameap-nodecmd` is refused those calls (with a
warning in the log naming the missing permission) until an operator grants
`node_commands`.

A refused call answers `plugin permission <name> required` in the
response's `error` field and is recorded in the audit log as `access.denied`
with the plugin as the actor; the module stays loaded.

### Path policy

Every path a plugin hands to `gameap-nodefs`, to `gameap-nodecmd` (`work_dir`)
or to a `HTTPResponse.file` reference is checked on the panel before it
reaches the daemon. In every mode a path containing a `..` segment or a NUL
byte is refused. `PLUGIN_NODEFS_PATH_POLICY` selects what else is allowed:

| Mode | Allowed paths |
|---|---|
| `unrestricted` (default) | anything the daemon itself permits — today's behaviour |
| `node_workpath` | inside the node's `work_path`; relative paths are resolved under it |
| `server_dirs` | inside the directory of a game server on that node (`work_path/<server dir>`, soft-deleted servers excluded); `work_dir` of a command must be absolute |

In both restricted modes the plugin's service directory (below) is allowed as well.

Every restricted mode additionally keeps one directory open: the plugin's own
**service directory**, `<work_path>/.plugins/<plugin id>` — the id being the
compact form the plugin's routes use. That is where a plugin belongs when it
needs scratch space on a node: staged request files, results a long node
command writes back, an archive built for download. It is per plugin and per
node, resolved from that node's own work path, so nothing has to be configured
and one plugin cannot reach another's files. A plugin loaded transiently (the
upload dry-run) has no id yet and gets no directory.

Put node-side working files there and the strictest mode still works. Put them
anywhere else under the work path and `server_dirs` refuses them.

`PLUGIN_NODEFS_ALLOWED_PATHS` (comma-separated absolute roots) widens the
restricted modes further, e.g. a shared `/opt/steamcmd`. It is a poor fit for
plugin working directories: the roots are absolute and apply to every node, so
a fleet with different work paths cannot be covered by one entry. Windows nodes are compared
case-insensitively with `\` normalised to `/`; symbolic links are not
resolved on the panel (the daemon still confines plugins to what it serves).
A refused call answers `path policy: <reason>: <path>` in the response's
`error` field (a `403` for file references) and is audited as `access.denied`
with reason `plugin_path_policy`, throttled to one record per minute per
function; the plugin is never disabled for it.

### Rate limits

The expensive host libraries are rate limited per plugin with a token bucket
(sustained calls per second plus a burst), configured by class:

| Class | Functions | Default |
|---|---|---|
| `nodecmd` | `gameap-nodecmd.ExecuteCommand` | 5/s, burst 20 |
| `servercontrol` | `gameap-servercontrol.*`, `gameap-daemontasks.CreateDaemonTask`, `gameap-servers.SaveServer` / `DeleteServer`, `gameap-serversettings.SaveServerSetting` | 5/s, burst 20 |
| `nodefs` | every `gameap-nodefs` operation | 50/s, burst 200 |
| `http` | `gameap-http.Fetch` | 20/s, burst 50 |
| `rbac` | every `gameap-rbac` operation | 10/s, burst 50 |
| `ssh` | every `gameap-ssh` operation, polling a running command included | 20/s, burst 60 |

`PLUGIN_RATELIMIT_<CLASS>_RPS` / `_BURST` tune them; RPS `0` disables a
class. A refused call answers `rate limited: gameap-<module> allows N calls/s
(burst M)` in the response's `error` field — the plugin is never disabled for
it — and is counted in the metrics; the audit record
(`plugin.hostcall.ratelimited`) is throttled to one per minute per function.
Buckets are per panel instance: with N instances the cluster-wide rate is N
times the limit.

### Audit trail

Privileged operations are written to the audit log with the plugin as the
actor (`auth_method=plugin`, `actor_id` = database id, `actor_login` =
compact id): `plugin.server.control` (action start/stop/restart/update/
install/reinstall), `plugin.server.save` / `plugin.server.delete`,
`plugin.server.setting`, `plugin.task.create`, `plugin.node.command`
(node, working directory and exit code — never the command text),
`plugin.node.file` (mkdir/copy/move/upload/remove/chmod/archive_*),
`plugin.ssh.connect` (host, port and user — never the credentials),
`plugin.ssh.exec` (connection and operation id — never the command text or
its stdin), `plugin.ssh.file` (path and connection — never the content), and
`plugin.rbac.role` / `plugin.rbac.grant` / `plugin.rbac.revoke`. Refusals
are `access.denied` with the plugin as the actor (reason
`plugin_permission_missing` or `plugin_path_policy`). Operator actions on a plugin are recorded with the
operator as the actor: `plugin.install`, `plugin.uninstall`,
`plugin.permissions.update` and `plugin.reloaded` with `trigger` = `manual`,
`auto` (recovery) or `sync` (another instance's change applied here). When the
plugin acted inside a user's request — an event raised by that request or a
plugin HTTP route — the user travels as `on_behalf_of_user_id` /
`on_behalf_of_login`; the same user reaches the plugin as
`PluginContext.user_id` on events and HTTP requests.

### Storage and cache quotas

`gameap-storage` is bounded per plugin: `PLUGIN_STORAGE_MAX_KEYS_PER_PLUGIN`
(10000), `PLUGIN_STORAGE_MAX_VALUE` (1 MiB) and
`PLUGIN_STORAGE_MAX_TOTAL` (64 MiB). A `Set` over a quota answers
`success=false` with the reason (`at most N storage entries per plugin`,
`payload exceeds N bytes`, `storage quota of N bytes exceeded`); replacing a
key releases its old payload first. `List` accepts an optional `limit` /
`offset` window and answers `has_more`; without a limit it keeps returning
every entry, as it always did.

A storage call the panel could not complete never surfaces the database
error: `Get`, `Set`, `Delete` and `List` answer with `error` set to a fixed
message (`failed to read storage entry` / `failed to store entry`) while the
cause goes to the panel log. Check `error` before trusting `found`,
`success` or `entries`.

`gameap-cache` keys live in a namespace of their own per plugin
(`plugin:<id>:`), so plugins never see each other's entries, and a value is
capped by `PLUGIN_CACHE_MAX_VALUE` (1 MiB). The cache is the panel's
cache backend: entries expire by TTL, are not deleted when the plugin is
uninstalled, and are only shared between panel instances when the backend
is (Redis or the database, not memory) — keep state that must survive a
reload or be visible to every instance in `gameap-storage`.

### Metrics

With `METRICS_TOKEN` set, `GET /metrics` (bearer token) exposes, per plugin
(label `plugin` = the compact id shown in the admin API):

- `gameap_plugin_host_calls_total{plugin,module,rpc,result}` and
  `gameap_plugin_host_call_duration_seconds{plugin,module}` — every host
  library function a guest invokes (`result` = `ok` | `panic`);
- `gameap_plugin_host_calls_denied_total{plugin,module,rpc,reason}` —
  refusals (`permission` | `rate_limit`);
- `gameap_plugin_guest_calls_total{plugin,export,result}` and
  `gameap_plugin_guest_call_duration_seconds{plugin,export}` — calls into the
  plugin (`ok` | `error` | `timeout` | `busy`);
- `gameap_plugin_events_dispatched_total{type,result}` (`handled` | `ignored`
  | `cancelled` | `error` | `dropped` | `denied` — the plugin lost
  `listen_events`) and `gameap_plugin_async_backlog` (fire-and-forget
  *batches* in flight or queued, not individual events);
- `gameap_plugin_disabled_total{plugin,reason}`,
  `gameap_plugin_memory_bytes{plugin}`, `gameap_plugin_enabled{plugin}`.

## Configuration

Plugins receive configuration during initialization:

```go
func (p MyPlugin) Initialize(ctx context.Context, req *proto.InitializeRequest) (*proto.InitializeResponse, error) {
    apiKey := req.Config["api_key"]
    webhookURL := req.Config["webhook_url"]

    if apiKey == "" {
        return &proto.InitializeResponse{
            Result: &proto.Result{
                Success: false,
                Error:   proto.String("api_key is required"),
            },
        }, nil
    }

    // Store configuration for later use
    p.apiKey = apiKey
    p.webhookURL = webhookURL

    return &proto.InitializeResponse{
        Result: &proto.Result{Success: true},
    }, nil
}
```

## Runtime limits and recovery

One plugin must not take the panel down, so the runtime bounds every module
and keeps the panel running when a plugin misbehaves. Everything below is
configurable through environment variables (`internal/config`).

### Loading

- A plugin that fails to load at startup (missing file, compile error,
  `Initialize` failure) is recorded with status `error` and the reason in
  `last_error`; the other plugins keep loading and the panel starts.
  `PLUGINS_STRICT_LOAD=true` restores the old behaviour (startup fails).
- Plugins with status `active` **and** `error` are attempted on every panel
  start — the cause may be gone. `disabled` is the operator's state and is
  never loaded; `updating` is skipped. The one exception is `PLUGINS_AUTOLOAD`:
  a plugin named there is the operator's explicit instruction and is set back
  to `active` at startup whatever its status (unchanged behaviour).
- `PLUGINS_CACHE_DIR` persists compiled wasm on local disk (keyed by module
  hash and wazero version) so restarts do not recompile every plugin;
  `PLUGINS_CACHE_ENABLED=false` turns caching off entirely.

### Limits

Sizes accept a unit suffix — `512K`, `64M`, `1G`; a plain number is bytes.
Durations use Go syntax (`30s`, `5m`). The unit lives in the value, not in the
variable name.

- `PLUGIN_RUNTIME_MAX_MEMORY` (256M) caps the linear memory of every module. A
  module that declares a larger maximum is clamped, not rejected; only a
  module whose *initial* memory already exceeds the cap fails to load, with
  an error naming both sizes. Standard Go builds reserve tens of MiB up front
  and grow their heap at runtime — raise the cap if such a plugin traps with
  out-of-memory.
- `PLUGIN_RUNTIME_MAX_MODULE_SIZE` (128M) rejects larger wasm files before
  compilation, for uploads, store installs and autoload alike.
- `PLUGIN_NODEFS_MAX_INLINE` (32 MiB) caps `gameap-nodefs` `Upload` payloads and
  what one `Download` may answer with. A `Download` that names an
  `Offset`/`Length` window is measured by the window, so a larger file stays
  readable one piece at a time.
- Guest `stdout` is forwarded to the panel log at debug level and `stderr` at
  warn level (attributes `plugin_id`, `stream`, `line`), so a Go/Rust panic
  message is visible. Lines are cut at 4 KiB and each stream is limited to
  200 lines per 10 seconds; drops are counted and reported.

### Disabled plugins and automatic recovery

A plugin is disabled at runtime when a guest call overruns its deadline
(event handler, HTTP route, scheduled task, archive callback — the runtime
closes the module) or when the guest terminates its own module (a Go `panic`
ends in `proc_exit`). The panel then:

1. records the reason on the plugin (`status=error`, `last_error`,
   `last_error_at`) and writes a `plugin.disabled` audit event;
2. reloads the plugin after `PLUGIN_RECOVERY_INITIAL_DELAY` (30s), doubling
   the wait on every further failure up to `PLUGIN_RECOVERY_MAX_DELAY` (10m).
   Each outcome is an audit event `plugin.reloaded` (`trigger=auto`). After
   `PLUGIN_RECOVERY_MAX_ATTEMPTS` (5) consecutive reloads the plugin stays in
   status `error` — `last_error` says so — until an operator reloads it or the
   panel restarts. A plugin that stayed healthy for longer than the maximum
   delay starts a fresh series. `PLUGIN_RECOVERY_ENABLED=false` keeps the
   disable permanent, as before — the status, reason and audit event are
   still recorded.

`GET /api/admin/plugins/loaded` lists every installed plugin with `status`,
`error`, `error_at`, `loaded` and `memory_bytes`; `POST
/api/admin/plugins/{id}/reload` reloads one on demand (it also cancels a
pending automatic reload), and the admin UI shows the status badge, the last
error and a Reload button.

Write your plugin accordingly: keep operation state in `gameap-storage`, not
in module globals, and make `Initialize` idempotent — a reload starts a fresh
module instance.

### Multi-instance

Several panel instances sharing one database each run their own module
instances. `loaded`, `enabled`, `memory_bytes` and `sync` in
`GET /api/admin/plugins/loaded` describe the instance that answered the
request, while `status` / `error` are the shared database record — the last
outcome written by any instance.

The database is the desired state, and every instance reconciles against it:
an install, update, uninstall, permission change or operator reload
performed on one instance is picked up by the others — at
once through the pubsub hint (`gameap:plugin:sync`, carrying only the plugin
id) and in any case on the periodic pass (`PLUGIN_SYNC_REFRESH_INTERVAL`,
60s). A reload is propagated through a `generation` counter on the record,
so "reload on A" restarts the plugin everywhere; a plugin whose file is
missing on an instance is re-downloaded from the store and verified against
the recorded checksum (plugins uploaded from a file need a shared
`FILES_DRIVER`, e.g. S3). A plugin that fails to load on one instance is
retried there with backoff (`PLUGIN_SYNC_MIN_BACKOFF` .. `PLUGIN_SYNC_MAX_BACKOFF`)
and reported as `sync.state = retrying` / `failed` with the local reason,
without touching the shared record. `PLUGIN_SYNC_DISABLED=true` restores the
previous behaviour (each instance only applies its own changes). Details:
`internal/services/pluginsync/README.md`.

## Example Plugin

See `pkg/plugin/examples/server-logger/` for a complete example plugin that logs server lifecycle events and registers a periodic `stats-report` scheduled task.

```bash
cd pkg/plugin/examples/server-logger
tinygo build -o server-logger.wasm -target=wasip1 -buildmode=c-shared .
```

## Directory Structure

```
pkg/plugin/
├── proto/
│   └── plugin.proto          # Plugin interface and event types
├── sdk/
│   ├── common/               # Shared types (Pagination, Sorting)
│   ├── servers/              # gameap-servers module
│   ├── users/                # gameap-users module
│   ├── nodes/                # gameap-nodes module
│   ├── games/                # gameap-games module
│   ├── gamemods/             # gameap-gamemods module
│   ├── daemontasks/          # gameap-daemontasks module
│   ├── serversettings/       # gameap-serversettings module
│   ├── servercontrol/        # gameap-servercontrol module
│   ├── nodefs/               # gameap-nodefs module (file operations)
│   ├── nodecmd/              # gameap-nodecmd module (command execution)
│   ├── cache/                # gameap-cache module
│   ├── crypto/               # gameap-crypto module (cryptography)
│   ├── authz/                # gameap-authz module (permission checks)
│   ├── rbac/                 # gameap-rbac module (roles & permissions)
│   ├── http/                 # gameap-http module
│   ├── net/                  # gameap-net module (host-managed socket I/O)
│   ├── protocol/             # ProtocolService (optional RCON/Query extension)
│   ├── scheduler/            # gameap-scheduler module (periodic tasks)
│   ├── secrets/              # gameap-secrets module (encrypted credentials)
│   ├── ssh/                  # gameap-ssh module (SSH to plugin-named hosts)
│   ├── host/                 # gameap-host module (own grants, host info)
│   └── log/                  # gameap-log module
├── examples/
│   ├── server-logger/        # Example plugin (lifecycle events)
│   └── protocol-extension/   # Example plugin (RCON/Query protocols)
├── manager.go                # Plugin manager
├── health.go                 # Runtime disable reasons and memory snapshot
├── hostmodules.go            # Records the host modules instantiated per plugin
├── observer.go               # Observer interface (guest/host call and event metrics)
├── hostcall_interceptor.go   # wazero decorator timing every host function a guest calls
├── guestlog.go               # Guest stdout/stderr → slog
├── runtimeconfig.go          # wazero runtime config (memory limit, cache)
├── cache.go                  # Compilation cache (in-memory / on disk)
├── dispatcher.go             # Event dispatcher
├── converters.go             # Domain → proto payload converters for events
├── wrapper.go                # WASM plugin wrapper
├── adapter.go                # ServerControl adapter
├── errors.go                 # Error definitions
└── README.md                 # This file
```

## Backward Compatibility Testing

A plugin compiled against an older panel must keep loading and working on
newer panels, and a plugin that imports host modules the panel does not
provide must be rejected cleanly instead of crashing at call time. The
`pkg/plugin/compatrust/` package guards both directions:

- Test plugins are Rust projects in the sibling `../test-plugins` repository.
  They are compiled to WebAssembly and committed here as gzipped fixtures
  under `pkg/plugin/compatrust/testdata/*.wasm.gz`.
- The tests load every fixture through `plugin.Manager` and assert the
  expected outcome: the plugin loads and answers calls, or it fails to load
  with a clear error.
- The CI workflow `.github/workflows/plugin-compat.yaml` runs the package on
  a matrix of panel versions: `HEAD`, the latest 4.4 release (`v4.4.1`) and
  the latest 4.3 release (`v4.3.5`). For the tagged legs the workflow
  overlays the test package from `HEAD` onto the tagged tree, so the same
  tests and fixtures run against every supported panel codebase.
  Version-specific test files are trimmed per leg (`*_v44_test.go` is
  dropped on 4.3.x, `compat_v43only_test.go` is dropped everywhere except
  4.3.x) — see `pkg/plugin/compatrust/README.md`.

Updating the fixtures after changing a test plugin:

```bash
cd ../test-plugins && make build
cp dist/*.wasm.gz <panel-repo>/pkg/plugin/compatrust/testdata/
```

Rules:

- A green matrix run is **mandatory** for any change to the plugin contracts
  in `pkg/plugin/proto` or `pkg/plugin/sdk`.
- When a new minor panel release is cut, add its latest patch tag to the
  workflow matrix and remove the tag of the version that goes EOL.
