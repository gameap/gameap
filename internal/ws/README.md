# WebSocket Infrastructure (`internal/ws`)

Shared WebSocket infrastructure for real-time browser communication.

## Architecture

```
Browser --WS--> Handler (upgrade) --> Client (read/write pumps)
                                          |
                                     Hub (topic-based fan-out)
                                          |
                                     Bridge (pub-sub subscriber)
                                          |
                                     Pub-Sub (realtime channels)
```

## Components

- **`message.go`** - JSON message envelope types (`InboundMessage`, `OutboundMessage`)
- **`client.go`** - Single WebSocket connection with read/write pumps and ping/pong heartbeat
- **`hub.go`** - Client registry with topic-based message fan-out
- **`bridge.go`** - Subscribes to pub-sub realtime channels and forwards events to the hub
- **`auth.go`** - Auth session extraction helpers

## Message Format

All WebSocket messages use a JSON envelope:

```json
{
  "type": "task.status",
  "payload": { ... },
  "ts": 1711234567
}
```

## Outbound filter

Frames delivered through `Hub.Broadcast` are already encoded by the time a `Client` sees
them, so a handler cannot post-process them by wrapping `SendMessage`. `Client.SetOutboundFilter`
installs a per-connection transform that runs on every outbound frame, whichever path it
came from. It must be installed before `Hub.Register` and `Run`.

The console endpoints use it to mask the server RCON password
(`internal/api/ws/base.NewOutboundMaskFilter`), which the game server prints as part of its
launch command line.

## Endpoints

- `GET /api/ws/tasks/{id}?token=<bearer>` - Real-time task status and output
- `GET /api/ws/servers/{server}/console?token=<bearer>` - Bidirectional server console
- `GET /api/ws/servers/{server}/attach?token=<bearer>` - Interactive PTY session
- `GET /api/ws/servers/{server}/file-manager/archive-operations?token=<bearer>` - Archive create/extract progress (`archive.progress` / `archive.complete` frames)

