# internal/audit

Structured security **audit log** for gameap-api.

Closes finding **C-3 / T-2** of [`docs/security/ASVS_L2.md`](../../docs/security/ASVS_L2.md)
and addresses OWASP ASVS 4.0.3 L2:

| ASVS | Requirement | Covered by |
| --- | --- | --- |
| 7.1.3 | Successful and failed authentication logged | `LoginSuccess`, `LoginFailure`, `TokenRejected`, `DaemonRejected` |
| 7.1.4 | Logs include enough context (actor, IP, ts, request_id) | `RequestContextMiddleware` + auto-enrichment |
| 7.2.1 | Security event logging | all helpers |
| 4.1.5 | Access-control failures logged | `AccessDenied` |

> **Not** covered here: 7.2.2 (remote/SIEM forwarding) and 7.3.1 (log
> integrity / append-only). These remain ASVS Sprint 3 items — records are
> emitted to the application `slog` sink (stdout) only, tagged
> `component=audit` so an operator can split/forward them downstream.

## How it works

1. `RequestContextMiddleware` wraps the **root** HTTP handler (one global
   integration point, see `internal/application/container.go`). It assigns
   or propagates a sanitized `X-Request-Id`, captures client IP / user
   agent / method / path into the context, and echoes the ID back in the
   response header.
2. Call sites invoke a helper (`audit.SensitiveOp`, `audit.SensitiveOpFailed`,
   `audit.AccessDenied`, `audit.TokenRejected`, …). Each helper builds an
   `Event`, auto-fills the actor from the request context (`pkg/auth` session →
   `session`/`pat`, daemon session → `daemon`, else `anonymous`) and the
   request metadata, then emits one `slog` record (message `"audit"`).
   `audit.SystemOp` is for work the panel does on its own, outside any request
   (actor `system`): `plugin.disabled` / `plugin.reloaded` from the plugin
   recovery supervisor. `audit.PluginOp` is for what a plugin does through the
   host libraries (actor `plugin`): `plugin.server.control`,
   `plugin.server.save` / `plugin.server.delete`, `plugin.server.setting`,
   `plugin.task.create`, `plugin.node.command`, `plugin.node.file`,
   `plugin.ssh.connect` / `plugin.ssh.exec` / `plugin.ssh.file`,
   `plugin.rbac.role` / `plugin.rbac.grant` / `plugin.rbac.revoke`, plus
   `access.denied` (reason `plugin_permission_missing`) and
   `plugin.hostcall.ratelimited` for refused calls — the latter two throttled
   to one record per minute per plugin and function so a looping plugin cannot
   flood the stream. When the plugin acted inside a user's request (an event
   or a plugin HTTP route) the user is recorded as `on_behalf_of_user_id` /
   `on_behalf_of_login`. `plugin.permissions.update` records an operator
   changing a plugin's grants (`granted` / `revoked`).

`audit.Logger` is the only interface (`contracts.go`). `NewLogger` wraps an
`*slog.Logger`; `NopLogger` is used when `AUDIT_ENABLED=false` and in tests.

## Record schema

Every record is a single `slog` line at `WARN` (failure/denied/blocked) or
`INFO` (success) with `component=audit` and a subset of:

```
event_type      stable token, e.g. auth.login.failure, access.denied, file.delete
category         authentication | authorization | ratelimit | *_op
outcome          success | failure | denied | blocked
actor_id         uint user/node id (omitted when anonymous)
actor_login      user login / node name
auth_method      session | pat | daemon | anonymous | system (panel-initiated, e.g. plugin recovery) | plugin (a plugin acting through a host library; actor_id is its database id, actor_login its compact id)
resource_type    server | node | user | token | plugin | file
resource_id      target id / path
action           delete | rename | role_assign | …
reason           stable failure token (never a raw error or secret)
request_id       correlation id (joins all records of one request)
ip, user_agent, http_method, path
```

## Adding a new audited operation

1. If a new kind, add an `EventType` constant in `event.go` (never
   repurpose an existing value — it is part of the contract).
2. At the **success / denial point** of the handler call
   `audit.SensitiveOp(ctx, h.audit, audit.EventX, audit.CategoryX, "<resource>", "<id>", "<action>", extra...)`.
3. Inject `audit.Logger` through the handler constructor and `router.go`.

### Redaction rules — enforced by review, not code

- Never log token values, passwords, private keys, or file contents.
- `reason` and `Extra` carry **stable, non-sensitive** tokens only.
- Submitted-but-unauthenticated identifiers go in `attempted_login`
  (`Extra`), never as `actor_login`.
