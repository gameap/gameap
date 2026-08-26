# gRPC, PubSub & WebSocket Architecture

## High-Level Overview

```mermaid
graph TB
    subgraph Browser["Browser (Frontend)"]
        WS_CLIENT["WebSocket Client"]
    end

    subgraph DAEMON["Game Daemon (Node)"]
        DAEMON_PROC["Daemon Process"]
    end

    subgraph API_SERVER["GameAP API Server"]

        subgraph HTTP_LAYER["HTTP Layer"]
            ROUTER["Router (chi)"]
            CONSOLE_WS["/api/ws/servers/{id}/console"]
            TASK_WS["/api/ws/tasks/{id}"]
        end

        subgraph WS_LAYER["WebSocket Layer (internal/ws)"]
            HUB["Hub<br/>topics → clients map"]
            CLIENT["Client<br/>readPump / writePump"]
            BRIDGE["Bridge<br/>PubSub → Hub adapter"]
        end

        subgraph GRPC_LAYER["gRPC Layer (internal/grpc)"]
            GATEWAY["DaemonGateway Service<br/>Connect (bidirectional stream)"]
            FILE_SVC["FileTransfer Service<br/>Upload / Download streams"]
            SESSION["Session<br/>per-daemon connection"]
            REGISTRY["SessionRegistry<br/>tracks all sessions"]

            subgraph HANDLERS["gRPC Handlers"]
                TASK_H["TaskHandler"]
                CMD_H["CommandHandler"]
                STATUS_H["ServerStatusHandler"]
            end
        end

        subgraph PUBSUB_LAYER["PubSub (internal/pubsub)"]
            PS_IFACE["PubSub Interface<br/>Publisher + Subscriber"]
            RETRY["RetryPublisher<br/>exp. backoff"]
            DLQ["DLQ Handler"]

            subgraph DRIVERS["Drivers"]
                MEM["Memory"]
                REDIS["Redis"]
                PG_PS["PostgreSQL<br/>LISTEN/NOTIFY"]
            end
        end

        subgraph DOMAIN["Domain / Repositories"]
            TASK_REPO["DaemonTaskRepository"]
            SERVER_REPO["ServerRepository"]
            NODE_REPO["NodeRepository"]
            CACHE_INV["CacheInvalidator"]
        end
    end

    %% Browser <-> WebSocket
    WS_CLIENT <-->|"WebSocket<br/>JSON messages"| ROUTER
    ROUTER --> CONSOLE_WS
    ROUTER --> TASK_WS
    CONSOLE_WS --> CLIENT
    TASK_WS --> CLIENT
    CLIENT <--> HUB

    %% Daemon <-> gRPC
    DAEMON_PROC <-->|"gRPC bidirectional stream<br/>(TLS / mTLS)"| GATEWAY
    DAEMON_PROC <-->|"gRPC streams<br/>Upload / Download"| FILE_SVC
    GATEWAY --> SESSION
    SESSION --> REGISTRY

    %% gRPC -> Handlers -> PubSub
    GATEWAY -->|"DaemonMessage"| TASK_H
    GATEWAY -->|"DaemonMessage"| CMD_H
    GATEWAY -->|"DaemonMessage"| STATUS_H

    TASK_H -->|"publish<br/>realtime:task:status:{id}<br/>realtime:task:output:{id}"| PS_IFACE
    CMD_H -->|"publish<br/>realtime:console:output:{id}<br/>realtime:console:result:{id}"| PS_IFACE
    STATUS_H -->|"update"| SERVER_REPO

    %% PubSub -> Bridge -> Hub -> Browser
    PS_IFACE -->|"subscribe<br/>realtime:task:*<br/>realtime:console:*"| BRIDGE
    BRIDGE -->|"Broadcast(topic, msg)"| HUB
    HUB -->|"send to subscribed clients"| CLIENT

    %% Console command flow (Browser -> Daemon)
    CLIENT -->|"console.command"| CONSOLE_WS
    CONSOLE_WS -->|"SendCommand"| REGISTRY
    REGISTRY -->|"local session"| SESSION
    REGISTRY -->|"remote dispatch<br/>daemon:command:dispatch:{nodeID}"| PS_IFACE

    %% Task dispatch
    REGISTRY -->|"remote dispatch<br/>daemon:task:dispatch:{nodeID}"| PS_IFACE
    PS_IFACE -->|"subscribe<br/>daemon:task:dispatch:*"| REGISTRY

    %% PubSub internal
    PS_IFACE --> RETRY --> DRIVERS
    RETRY -.->|"on failure"| DLQ

    %% Cache
    PS_IFACE -->|"subscribe<br/>cache:invalidate:*"| CACHE_INV

    %% Domain
    TASK_H --> TASK_REPO
    GATEWAY --> NODE_REPO
    GATEWAY --> TASK_REPO

    classDef browser fill:#4A90D9,stroke:#2C5F8A,color:#fff
    classDef daemon fill:#E67E22,stroke:#BA6318,color:#fff
    classDef ws fill:#27AE60,stroke:#1E8449,color:#fff
    classDef grpc fill:#8E44AD,stroke:#6C3483,color:#fff
    classDef pubsub fill:#E74C3C,stroke:#C0392B,color:#fff
    classDef domain fill:#95A5A6,stroke:#7F8C8D,color:#fff

    class WS_CLIENT browser
    class DAEMON_PROC daemon
    class HUB,CLIENT,BRIDGE,CONSOLE_WS,TASK_WS ws
    class GATEWAY,FILE_SVC,SESSION,REGISTRY,TASK_H,CMD_H,STATUS_H grpc
    class PS_IFACE,RETRY,DLQ,MEM,REDIS,PG_PS pubsub
    class TASK_REPO,SERVER_REPO,NODE_REPO,CACHE_INV domain
```

## Data Flow: Console Command (Full Cycle)

```mermaid
sequenceDiagram
    participant B as Browser
    participant WS as WebSocket Client
    participant Hub as WS Hub
    participant CH as Console Handler
    participant Reg as SessionRegistry
    participant S as gRPC Session
    participant D as Daemon
    participant CmdH as CommandHandler
    participant PS as PubSub
    participant Bridge as WS Bridge

    B->>WS: {"type":"console.command", "payload":{"command":"say hello"}}
    WS->>CH: readPump -> MessageHandler
    CH->>CH: check AbilityNameGameServerConsoleSend
    CH->>Reg: SendCommand(nodeID, CommandRequest)

    alt Daemon connected locally
        Reg->>S: Stream.Send(GatewayMessage)
        S->>D: gRPC bidirectional stream
    else Daemon on another instance
        Reg->>PS: publish daemon:command:dispatch:{nodeID}
        PS-->>Reg: (other instance) handleCommandDispatch
        Reg->>S: Stream.Send(GatewayMessage)
        S->>D: gRPC bidirectional stream
    end

    D->>D: Execute command
    D->>S: DaemonMessage(CommandOutput)
    S->>CmdH: HandleCommandOutput()
    CmdH->>PS: publish realtime:console:output:{serverID}
    PS->>Bridge: handler (subscribed to realtime:console:*)
    Bridge->>Hub: Broadcast("realtime:console:output:{serverID}", msg)
    Hub->>WS: send to subscribed clients
    WS->>B: {"type":"console.output", "payload":{"chunk":"..."}}

    D->>S: DaemonMessage(CommandResult)
    S->>CmdH: HandleCommandResult()
    CmdH->>PS: publish realtime:console:result:{serverID}
    PS->>Bridge: handler
    Bridge->>Hub: Broadcast
    Hub->>WS: send
    WS->>B: {"type":"console.result", "payload":{"exitCode":0}}
```

## Data Flow: Task Execution

```mermaid
sequenceDiagram
    participant API as API / TaskDispatcher
    participant Reg as SessionRegistry
    participant PS as PubSub
    participant S as gRPC Session
    participant D as Daemon
    participant TH as TaskHandler
    participant Bridge as WS Bridge
    participant Hub as WS Hub
    participant B as Browser (WS /tasks/{id})

    API->>Reg: DispatchTask(nodeID, DaemonTask)

    alt Local session
        Reg->>S: Stream.Send(GatewayMessage{DaemonTask})
    else Remote instance
        Reg->>PS: publish daemon:task:dispatch:{nodeID}
        PS-->>Reg: other instance -> local session
        Reg->>S: Stream.Send
    end

    S->>D: gRPC stream -> task

    loop Task execution
        D->>S: TaskStatusUpdate(status=running)
        S->>TH: HandleTaskStatus()
        TH->>PS: publish realtime:task:status:{taskID}
        PS->>Bridge: handler
        Bridge->>Hub: Broadcast
        Hub->>B: {"type":"task.status"}

        D->>S: TaskOutput(chunk)
        S->>TH: HandleTaskOutput()
        TH->>PS: publish realtime:task:output:{taskID}
        PS->>Bridge: handler
        Bridge->>Hub: Broadcast
        Hub->>B: {"type":"task.output"}
    end

    D->>S: TaskStatusUpdate(status=success)
    S->>TH: HandleTaskStatus -> publishTaskComplete
    TH->>PS: publish realtime:task:status:{taskID}
    PS->>Bridge: handler
    Bridge->>Hub: Broadcast
    Hub->>B: {"type":"task.complete"}
```

## PubSub Channels

| Category | Channel | Publisher | Subscriber |
|----------|---------|-----------|------------|
| Console | `realtime:console:output:{serverID}` | CommandHandler | WS Bridge |
| Console | `realtime:console:result:{serverID}` | CommandHandler | WS Bridge |
| Task | `realtime:task:status:{taskID}` | TaskHandler | WS Bridge |
| Task | `realtime:task:output:{taskID}` | TaskHandler | WS Bridge |
| Dispatch | `daemon:task:dispatch:{nodeID}` | SessionRegistry | SessionRegistry (other instance) |
| Dispatch | `daemon:command:dispatch:{nodeID}` | SessionRegistry | SessionRegistry (other instance) |
| Cache | `cache:invalidate:*` | Application | CacheInvalidator |
| Session | `daemon:session:connected` | SessionRegistry | - |
| Session | `daemon:session:closed` | SessionRegistry | - |
| Plugin | `plugin:sync` | Plugin admin handlers (install, update, uninstall, reload, permissions, config) | pluginsync reconciler (every instance; the payload is only a hint — the plugin table is re-read) |

All channels are prefixed with `gameap:` (e.g., `gameap:realtime:task:status:{id}`). The WS Bridge strips this prefix when converting to WebSocket topics.

## Key Components

| Component | Location | Role |
|-----------|----------|------|
| `Hub` | `internal/ws/hub.go` | Routes messages by topic to WebSocket clients |
| `Client` | `internal/ws/client.go` | Single WS connection (readPump + writePump) |
| `Bridge` | `internal/ws/bridge.go` | Adapter from PubSub to Hub, strips `gameap:` prefix |
| `Session` | `internal/grpc/session/session.go` | Single gRPC connection with a daemon |
| `SessionRegistry` | `internal/grpc/session/registry.go` | Registry of all sessions + cross-instance dispatch via PubSub |
| `DaemonGateway` | `internal/grpc/gateway/service.go` | Bidirectional streaming gRPC service |
| `FileTransferService` | `internal/grpc/filetransfer/service.go` | File upload/download streaming gRPC service |
| `TaskHandler` | `internal/grpc/handlers/task_handler.go` | Processes task status/output from daemons |
| `CommandHandler` | `internal/grpc/handlers/command_handler.go` | Processes command output/results from daemons |
| `ServerStatusHandler` | `internal/grpc/handlers/server_status_handler.go` | Processes server status batch updates |
| `PubSub` | `internal/pubsub/pubsub.go` | Interface with Memory / Redis / PostgreSQL drivers |
| `RetryPublisher` | `internal/pubsub/retry/publisher.go` | Wrapper with exponential backoff + DLQ |

## gRPC Services

### DaemonGateway (`pkg/proto/gateway.proto`)

- `Connect(stream DaemonMessage) returns (stream GatewayMessage)` — bidirectional stream for persistent daemon connections
- `Enroll(EnrollRequest) returns (EnrollResponse)` — daemon enrollment (no auth required)

**DaemonMessage types** (daemon -> server): `RegisterRequest`, `Heartbeat`, `TaskStatusUpdate`, `TaskOutput`, `CommandOutput`, `CommandResult`, `ServerStatusBatch`

**GatewayMessage types** (server -> daemon): `RegisterAck`, `DaemonTask`, `TaskCancel`, `CommandRequest`, `ServerConfigBatch`, `ShutdownNotification`

### FileTransferService (`pkg/proto/filetransfer.proto`)

- `UploadFile(stream UploadChunk) returns (UploadResult)` — client streaming upload with SHA256 checksum
- `DownloadFile(DownloadRequest) returns (stream DownloadChunk)` — server streaming download
- `FileOperation(FileOperationRequest) returns (FileOperationResponse)` — delete, move, copy, chmod, mkdir, touch, stat, exists
- `ListDirectory(ListDirectoryRequest) returns (ListDirectoryResponse)` — directory listing

## PubSub Drivers

| Driver | Backend | Use Case |
|--------|---------|----------|
| Memory | In-process | Testing, single-instance deployments |
| Redis | Redis PUBLISH/SUBSCRIBE | Multi-instance with Redis available |
| PostgreSQL | LISTEN/NOTIFY | Multi-instance without Redis (7900 byte payload limit) |

Configured via `config.PubSub.Driver`. Optional retry wrapper (`config.PubSub.Retry`) and DLQ handler (`config.PubSub.DLQ`).

## Server Startup Sequence

1. Create container (lazy-initializes all services)
2. Run migrations, seed database; the pluginsync reconciler subscribes to `plugin:sync`, then plugins are loaded and the first reconcile pass runs
3. `startPubSub()`:
   - CacheInvalidator subscribes to `cache:invalidate:*`
   - WS Bridge subscribes to `realtime:task:*` and `realtime:console:*`
   - PubSub listener starts (blocks in goroutine)
4. gRPC server starts — SessionRegistry subscribes to `daemon:task:dispatch:*`
5. HTTP server starts (shares port with gRPC via `cmux` multiplexer)

## Authentication

- **gRPC**: API key (`x-api-key` + `x-node-id` metadata) or mTLS
- **WebSocket**: Session-based auth via HTTP middleware + RBAC permission checks
- **Enroll RPC**: Public (no auth)
