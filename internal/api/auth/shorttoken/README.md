# shorttoken

`POST /api/auth/short-lived-token` — issues a **single-use, short-lived**
authorization token (TTL ≤ 10s) for the authenticated caller.

## Why

The browser cannot set an `Authorization` header on a WebSocket upgrade, so
the token has to go in the URL (`?token=`). A long-lived token (24h/7d) leaking
into nginx/proxy logs, browser history or the network inspector is a serious
exposure. This endpoint mints a throwaway ticket instead: it lives seconds and
is destroyed by the auth middleware on first successful use, so a captured copy
is worthless once the connection is established. The same applies to file
downloads where a token may end up in a URL.

## Model

- Format: `glst_<48-char secret>`. The prefix is distinct from PASETO/JWT/PAT
  so `middlewares/auth.go` routes it by prefix.
- Stored in the cache under `auth:shorttoken:<sha256(secret)>` — only the hash
  is ever stored, never the secret (mirrors the Personal Access Token model).
- The cached payload (`pkg/auth.ShortLivedPayload`) carries the user identity
  and, for PAT-derived sessions, the PAT id + abilities, so the reconstructed
  session is no broader than the session that minted it.
- The minting request must use the long-lived token **in the header**. The
  route disallows guest access and rejects short-lived tokens, so a short
  token cannot be used to mint another.

## Scope

A short-lived session is accepted **only** on routes that opt in via
`AllowShortLivedToken: true` (the WebSocket endpoints and the file-download
endpoints). Everywhere else `ShortLivedScopeMiddleware` rejects it with 403.

Note: the scope guard is wired into the main route table only. Plugin routes
(`/api/plugins/...`, `plugins.js`, `plugins.css`) are not covered; a leaked
single-use token could in principle be replayed there within its ≤10s window.
This is an accepted residual given single-use + sub-second client consumption;
revisit if plugin endpoints ever accept a token in the URL.

## Deployment

The token lives in `cache.Cache`. With more than one API instance the cache
**must** be shared (Redis or a database backend) — with the in-memory cache a
token issued by one instance cannot be validated by another, so the WebSocket
connecting through a load balancer would fail. This is the same constraint
already documented for `pkg/auth/revocation_cache.go`.
