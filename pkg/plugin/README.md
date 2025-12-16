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
- **Hook into server lifecycle events** (pre/post start, stop, restart, install, update, reinstall, delete)
- **Access repositories** (servers, users, nodes, games, tasks, settings)
- **Control servers** (start, stop, restart, update, install)
- **Use caching** (get, set, delete)
- **Make HTTP requests** (external API calls)
- **Log messages** (debug, info, warn, error)
- **Register custom HTTP endpoints** (extend the API)

## Event Types

| Event | Trigger | Cancellable |
|-------|---------|-------------|
| `SERVER_PRE_START` | Before server start task created | Yes |
| `SERVER_POST_START` | After server start task created | No |
| `SERVER_PRE_STOP` | Before server stop task created | Yes |
| `SERVER_POST_STOP` | After server stop task created | No |
| `SERVER_PRE_RESTART` | Before server restart task created | Yes |
| `SERVER_POST_RESTART` | After server restart task created | No |
| `SERVER_PRE_INSTALL` | Before server install task created | Yes |
| `SERVER_POST_INSTALL` | After server install task created | No |
| `SERVER_PRE_UPDATE` | Before server update task created | Yes |
| `SERVER_POST_UPDATE` | After server update task created | No |
| `SERVER_PRE_REINSTALL` | Before server reinstall workflow | Yes |
| `SERVER_POST_REINSTALL` | After server reinstall workflow | No |
| `SERVER_PRE_DELETE` | Before server delete task created | Yes |
| `SERVER_POST_DELETE` | After server delete task created | No |
| `SERVER_CREATED` | After server created in database | No |
| `SERVER_UPDATED` | After server updated in database | No |
| `SERVER_DELETED` | After server deleted from database | No |
| `DAEMON_TASK_CREATED` | After daemon task created | No |
| `DAEMON_TASK_COMPLETED` | After daemon task completed successfully | No |
| `DAEMON_TASK_FAILED` | After daemon task failed | No |

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

Provides caching capabilities.

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

Provides file system operations on daemon nodes.

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

// Download a file
downloadResp, _ := fs.Download(ctx, &nodefs.DownloadRequest{
    NodeId: 1,
    Path:   "/home/servers/server.cfg",
})
content := downloadResp.Content

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
```

### gameap-nodecmd

Provides command execution on daemon nodes.

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

```go
nodesRepo := nodes.NewNodesService()

nodeResp, _ := nodesRepo.GetNode(ctx, &nodes.GetNodeRequest{Id: 1})
if nodeResp.Found {
    node := nodeResp.Node
}
```

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

### Cancelling Events

For `PRE_*` events, plugins can prevent the operation by setting `ShouldCancel`:

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

## Security

- Plugins run in a WebAssembly sandbox
- No direct filesystem access
- No direct network access (use gameap-http for external calls)
- Repository operations respect RBAC permissions
- Plugin configuration can restrict capabilities

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

## Example Plugin

See `pkg/plugin/examples/server-logger/` for a complete example plugin that logs server lifecycle events.

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
│   ├── http/                 # gameap-http module
│   └── log/                  # gameap-log module
├── examples/
│   └── server-logger/        # Example plugin
├── manager.go                # Plugin manager
├── dispatcher.go             # Event dispatcher
├── wrapper.go                # WASM plugin wrapper
├── adapter.go                # ServerControl adapter
├── errors.go                 # Error definitions
└── README.md                 # This file
```
