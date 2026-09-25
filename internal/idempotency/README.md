# internal/idempotency

`Idempotency-Key` support for mutating API routes, after the IETF draft
[`draft-ietf-httpapi-idempotency-key-header`](https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/).
A billing integration that retries a timed-out `POST /api/servers` with the
same key gets the original `201` back instead of a second server.

## Contract

- The header is optional. Without it a route behaves as before.
- Only routes registered with `AllowIdempotencyKey: true` in
  `internal/api/router.go` honour it. Elsewhere it is ignored.
- Key: an RFC 8941 sf-string (`"..."`) or a bare token, 1–255 printable ASCII
  characters. A malformed key or a repeated header → `400`. An empty value
  counts as absent.
- Keys are scoped to the user (`session.User.ID`), not the token. A PAT rotated
  between an attempt and its retry still gets the replay, and one user's key
  never returns another user's response.
- Fingerprint: method, escaped path, sorted query and the exact body bytes.
  Keep the serialized body with its key for retries, including JSON field
  order and whitespace. JSON is not canonicalized: generic maps and typed
  handler inputs differ in their treatment of duplicate or case-insensitive
  field names.

| Situation                                        | Response                                                                                                                   |
| ------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------- |
| First request                                    | Runs the handler; its response is stored                                                                                   |
| Retry after completion                           | Stored status and body, `Content-Type`/`Location`/`ETag`/`Last-Modified`/`Cache-Control`, plus `Idempotent-Replayed: true` |
| Same key, different request (body, path, method) | `422`                                                                                                                      |
| Original still running                           | `409` + `Retry-After: 1`                                                                                                   |
| Retry after access to the server was revoked     | `403`/`404` from the handler's access check; the stored response is not returned                                           |
| Store, lock or replay access check unavailable   | `503` + `Retry-After: 5`; the handler is not called                                                                        |
| Body over 1 MiB                                  | `413`                                                                                                                      |

- **Every outcome the handler produced is stored: 2xx, 4xx and 5xx, panics
  included.** A failure may come after a change was made, so running the
  request again could make it twice. A `500` is "indeterminate". The client
  checks the state and, if needed, retries with a new key. A `4xx` is replayed
  as well: after fixing the request or the server-side state, send it with a
  new key.
- Network failures and gateway `502`/`504` responses do not prove that the
  operation failed. Keep the key and original body until its outcome is known;
  creating a new key may execute the operation again.
- Not stored: requests rejected before the handler (authentication, PAT
  abilities, admin checks — they wrap this middleware), this middleware's own
  `400/409/413/422/503`, `http.ErrAbortHandler`, and a response over 1 MiB.
  A retry of those runs the handler.
- Outcomes are replayed for `IDEMPOTENCY_KEY_TTL` (24h); after that the key is
  new again.
- A request with a key **runs to completion even if its client disconnects**.
  The handler gets `context.WithoutCancel` capped at 2 minutes. Otherwise a
  client timeout would record `context canceled` as the outcome of a request
  that may already have made a change.

### The remaining window

The outcome is stored after the handler returns, not in the handler's
transaction. If the process dies between the handler's change and that write,
the lock expires (1 minute) and a retry runs the request again.

## Rule for new routes

**Never enable `AllowIdempotencyKey` on a route whose response carries a
secret or personal data** (tokens, TOTP secrets, recovery codes, setup keys,
RCON output with cvar values and player IPs). The response is stored for a
day. For the same reason `auth`, `tokens`, `profile` and `2fa` routes and
`POST /api/servers/{server}/rcon` are excluded. Multipart uploads and
streaming routes are excluded because of their body and response sizes.

## Drivers (`IDEMPOTENCY_DRIVER`)

- `database` (default): table `idempotency_keys`, with database locks in
  `kv_store` independently of the cache driver. Clearing or evicting the cache
  cannot release an active idempotency lock. `Janitor` deletes expired rows every
  `IDEMPOTENCY_JANITOR_INTERVAL`.
- `redis`: `RedisStore`, atomic `SET NX` with absolute millisecond expiry in its own database
  (`IDEMPOTENCY_REDIS_DB`, default 2). The lock is on the same client.
  `cache.Redis.Clear()` is `FLUSHDB`, so with `CACHE_DRIVER=redis` the
  container refuses to start when the database is the cache's.
  `SharesDatabase` decides it by a probe key written through one client and
  looked up through the other: addresses cannot tell, since `localhost`,
  `127.0.0.1` and a DNS alias may name one server.
  Redis must not evict keys before they expire (`maxmemory-policy noeviction`);
  `WarnIfEvictable` logs a warning at startup otherwise. The policy is
  instance-wide, so the store needs a Redis instance of its own, not the
  cache's.
- `none`: `Disabled()`; routes are not wrapped, the header is ignored, nothing
  is stored.

## Security

- The client key and the fingerprint are stored as HMAC-SHA256 under a key
  derived from `AUTH_SECRET`. Request bodies (passwords) and raw keys (which
  may be derived from customer data) never reach the store.
- The middleware sits innermost, inside authentication, PAT abilities and
  admin checks. Server handlers implement `AuthorizeIdempotencyReplay` to
  repeat their object-level access checks before a stored response is returned.
  A replay is refused when access to the server or the required ability has
  been revoked, or when the server is blocked for the user. The check's 4xx is
  returned as is; a check that cannot run (database or RBAC failure) is
  answered with `503`, like an unavailable store.
