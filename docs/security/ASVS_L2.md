# OWASP ASVS 4.0.3 — Level 2 (Standard) Conformance Audit

Self-assessment of the **gameap-api** project against the
[OWASP Application Security Verification Standard 4.0.3](https://owasp.org/www-project-application-security-verification-standard/),
**Level 2 (Standard)**.

L2 is the next maturity step after the existing
[Level 1 baseline](./ASVS.md). It is the typical target for applications
that handle sensitive data, host control-plane functionality, or expose
authenticated APIs to non-trivial attacker populations — all of which apply
to a self-hosted game server panel that manages user accounts, daemon
credentials, file operations on game hosts, and a public HTTP + gRPC
attack surface.

---

## 1. Scope and intent

| Item | Value |
| --- | --- |
| Standard | OWASP ASVS 4.0.3 |
| Target verification level | **L2 (Standard)** |
| Application type | Self-hosted REST/JSON API + gRPC daemon control plane (with embedded SPA + WebSocket) |
| Primary trust boundaries | (a) public Internet → API HTTP/WS server; (b) game daemon → API gRPC bidi + legacy daemon HTTP API; (c) operator → admin endpoints; (d) plugin (WASM) → host runtime |
| L1 baseline | [`docs/security/ASVS.md`](./ASVS.md) (last reviewed 2026-04-28) |
| Last reviewed | 2026-05-15 |
| Owners | gameap-api maintainers |
| Audit method | Static review of source + test suite; no penetration testing engagement |

### Out of scope

- The Vue 3 SPA in `web/frontend/` (front-end concerns; should get its own
  ASVS audit if browser hardening matters).
- Operational concerns not visible in source (incident response, secure
  SDLC pipelines, vulnerability disclosure programme).
- Deep analysis of the `wazero` WASM runtime internals; only host
  capabilities exposed via `internal/plugin/hostlibrary` are covered.

### How to read each entry

| Symbol | Meaning |
| --- | --- |
| ✅ Met | Implemented with automated test evidence or single vetted enforcement point |
| 🟡 Partial | Implemented but with caveats (operator-config dependent, scope limited, missing edge cases) |
| ❌ Not met | Gap — see Roadmap §7 |
| ➖ N/A | Requirement does not apply to a JSON/HTTP + gRPC API of this type |

Evidence is given as `path/to/file.go:line` plus the relevant test name
when one exists.

---

## 2. Executive summary

### 2.1 Aggregate L2 conformance

Counted over the **L2-applicable** requirements only (N/A excluded). The
score weights Partial at 50% of Met:

```
score = Met / (Met + Partial * 0.5 + Not met)
```

| Chapter | Met | Partial | Not met | N/A | Chapter score |
| --- | ---:| ---:| ---:| ---:| ---:|
| V1 Architecture & threat modeling | 3 | 4 | 6 | — | 38% |
| V2 Authentication | 14 | 5 | 13 | 4 | 51% |
| V3 Session management | 8 | 4 | 3 | 2 | 67% |
| V4 Access control | 6 | 2 | 3 | — | 64% |
| V5 Validation / encoding | 13 | 2 | 1 | 6 | 88% |
| V6 Stored cryptography | 5 | 4 | 3 | — | 58% |
| V7 Error handling & logging | 4 | 3 | 5 | — | 46% |
| V8 Data protection | 3 | 3 | 4 | 1 | 45% |
| V9 Communications | 3 | 3 | 2 | — | 56% |
| V10 Malicious code | 1 | 2 | 0 | 4 | 67% |
| V11 Business logic | 4 | 2 | 2 | — | 63% |
| V12 Files & resources | 9 | 2 | 4 | 1 | 67% |
| V13 API & web service | 7 | 2 | 2 | 2 | 73% |
| V14 Configuration | 8 | 5 | 6 | — | 55% |
| **Total** | **88** | **43** | **53** | **20** | **57%** |

**L2 conformance: ~57%** — the project has a strong testing baseline,
solid AuthN/AuthZ enforcement, and good defaults around cryptographic
primitives, but is held back by missing security HTTP headers, no
audit logging, no MFA, and a handful of high-severity gaps documented
in §3. Realistic estimate: **~80% achievable in 3 sprints**, full L2
conformance after the MFA + threat-model deliverables (§7).

### 2.2 Strengths worth preserving

The project ships meaningful security plumbing that many comparable
panels lack:

- **Server-scoped IDOR enforcement** at a single point
  (`internal/api/servers/base/serverfinder.go:30-57`) with 7 unit + 3
  fuzz tests (`router_security_idor_*_test.go`).
- **Login brute-force protection** via `LoginRateLimitMiddleware`
  (`internal/api/middlewares/login_ratelimit.go`) — 20/IP, 5/username
  per 15-min sliding window, 9 unit tests covering edge cases.
- **PASETO v4.local** (authenticated encryption) instead of plain JWT
  for primary session tokens (`pkg/auth/paseto.go`).
- **mTLS for daemon gRPC** with constant-time peer verification
  (`internal/grpc/interceptors/auth.go:102-137`).
- **Constant-time comparisons** consistently used everywhere a secret
  is compared (PAT `auth.go:255`, daemon `daemon.go:44`, setup key
  `setup_key.go:42`, gRPC API key `grpc/interceptors/auth.go:185`).
- **Token revocation** via denylist (`pkg/auth/revocation*.go`) +
  enforced on every request post-auth.
- **SQL identifier allow-listing** for sort fields
  (`internal/filters/order.go::ParseUserSort`) with 16 test cases
  including `id;DROP TABLE users--`.
- **File-upload path canonicalisation** + per-server scoping +
  fuzz coverage (`FuzzFileManagerPath_*`).
- **CORS scheme awareness** — `deriveDefaultOrigin` tracks
  `TLS.ForceHTTPS` so HTTPS deployments do not advertise an `http://`
  origin (`internal/api/middlewares/cors.go:33-50`).
- **Weekly fuzz workflow** (`.github/workflows/security-fuzz.yaml`)
  running 9 fuzz targets, auto-files GitHub issues on crash.
- **Daemon API token migration to SHA-256** with idempotent re-run
  guard (`migrations/postgres/007_hash_daemon_api_tokens.go`).
- **Recovery middleware** with stack-trace logged server-side only
  (`internal/api/middlewares/recovery.go`).

### 2.3 Top-10 L2 gaps (impact-ranked)

| # | Gap | ASVS req | Severity |
| --- | --- | --- | --- |
| T-1 | No security HTTP headers (HSTS, X-CTO, X-Frame-Options, CSP, Referrer-Policy) | 14.3.2, 14.4.6, 14.4.7 | **High** |
| T-2 | No structured audit log (auth failures, AC denials, sensitive ops) | 7.1.3, 7.2.1, 7.2.2 | **High** |
| T-3 | Legacy daemon outbound TLS allows TLS 1.0 + `InsecureSkipVerify=true` | 9.1.2, 9.2.1 | **High** |
| T-4 | No MFA (TOTP / WebAuthn / OOB) for users or admins | 2.7.x, 2.8.x, 4.3.1 | High |
| T-5 | `node.GdaemonAPIKey` stored plaintext in DB (used per-request by gRPC) | 2.10.2, 6.1.3, 8.3.4 | High |
| T-6 | No password policy (min length, breached check, max enforced) | 2.1.1, 2.1.7 | Medium |
| T-7 | No idle session timeout (only absolute 24h) | 3.3.2 | Medium |
| T-8 | bcrypt cost stuck at `DefaultCost`=10 (L2 wants ≥13) | 2.4.4 | Medium |
| T-9 | No file-upload MIME / magic-byte verification | 12.4.1, 12.4.2 | Medium |
| T-10 | No `govulncheck` / SBOM in CI | 14.2.2, 14.2.3, 14.2.4 | Medium |

---

## 3. Critical findings

These are concrete issues uncovered during the audit that go beyond
"unimplemented L2 requirement" — each is a potential or actual security
weakness the project should address regardless of certification ambitions.
Severity uses CVSS-style reasoning weighted by realistic exploitability
in this deployment model.

---

### C-1 · **High** · Legacy daemon outbound TLS allows TLS 1.0 + skips cert verification

| | |
| --- | --- |
| File | `internal/daemon/conn.go:93-99` |
| CWE | CWE-295 (Improper Certificate Validation), CWE-326 (Inadequate Encryption Strength) |
| ASVS | 9.1.2 (Strong TLS configuration), 9.2.1 (Certificate validation) |

The legacy HTTP client used to talk to the daemon's plaintext-tag HTTP
API is constructed with:

```go
tlsConfig := &tls.Config{
    RootCAs:            serverCertPool,
    Certificates:       certificates,
    InsecureSkipVerify: true, //nolint:gosec
    MinVersion:         tls.VersionTLS10,
    MaxVersion:         tls.VersionTLS13,
}
```

The `//nolint:gosec` suppression silences the static analyser. Even
though `RootCAs` is populated, `InsecureSkipVerify: true` overrides it —
the client will accept any presented certificate. TLS 1.0 has been
formally deprecated since RFC 8996 (2021) and breaks the L2 baseline
which mandates TLS 1.2+.

**Impact**: an attacker positioned between the panel and a daemon node
(e.g., compromised intermediate router, malicious node in a multi-tenant
network, MITM on a misconfigured private network) can read and modify
all traffic on this legacy channel — including server console commands
and file upload streams that still flow through the legacy path. The
attacker does not need any node credential.

**Remediation**: raise `MinVersion` to `tls.VersionTLS12`, remove
`InsecureSkipVerify` (rely on `RootCAs` + `tls.Dialer`), and add a
config flag (e.g. `DAEMON_LEGACY_INSECURE_TLS=true`) for the legacy
self-signed cert scenario so the default is secure. Cover with an
integration test that asserts cert validation actually runs.

---

### C-2 · **High** · No security HTTP headers anywhere on the response path

| | |
| --- | --- |
| Files | `internal/api/middlewares/` (no such middleware exists); `pkg/api/responder.go:96-100` |
| CWE | CWE-693 (Protection Mechanism Failure) |
| ASVS | 14.3.2 (Security headers), 14.4.6 (Anti-clickjacking), 14.4.7 (`X-Content-Type-Options: nosniff`), 8.2.1 (`Cache-Control`) |

```bash
$ grep -rln "Strict-Transport-Security\|X-Content-Type-Options\|X-Frame-Options\|Content-Security-Policy\|Referrer-Policy" \
        internal/api/middlewares/ pkg/api/
# (no matches — outside of vite's dev server)
```

The panel ships a Vue SPA, exposes JSON API responses, serves
WebSocket upgrades, and accepts file uploads — none of which are
returned with any of the standard browser-side defenses.

**Impact**:
- Without `Strict-Transport-Security`, the first TLS connection is
  downgrade-attackable.
- Without `X-Content-Type-Options: nosniff`, MIME-sniffed file uploads
  can render as HTML and execute JS in the same origin.
- Without `X-Frame-Options` / `CSP frame-ancestors`, the panel can be
  framed by a malicious site (clickjack → "click here to delete server").
- Without `Cache-Control: no-store` on auth/sensitive responses, a
  shared cache (intermediate proxy, browser back/forward cache) can
  return another user's response.

**Remediation**: add a single `SecurityHeadersMiddleware` near the top
of the chain in `internal/api/router.go` and a `pkg/api/responder.go`
addition for `Cache-Control: no-store, no-cache, must-revalidate`,
`Pragma: no-cache` on `/api/auth/*`. Suggested values:

```
Strict-Transport-Security: max-age=31536000; includeSubDomains
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: strict-origin-when-cross-origin
Content-Security-Policy: default-src 'self'; frame-ancestors 'none'; …
```

CSP requires the SPA to use no inline scripts/styles — verify with a
report-only deploy first.

---

### C-3 · **High** · No structured audit log for security events

| | |
| --- | --- |
| Files | `internal/api/middlewares/auth.go` (no `slog` on rejection branches), `internal/api/middlewares/personal_access.go`, `internal/api/middlewares/daemon.go` |
| CWE | CWE-778 (Insufficient logging), CWE-223 (Omission of security-relevant information) |
| ASVS | 7.1.3 (Successful/failed auth logged), 7.1.4 (Sufficient context), 7.2.1 (Security event logging), 7.2.2 (Log forwarding), 4.1.5 (AC failure logging) |

The auth middleware returns 401/403 on every failure path without ever
calling `slog`. Same in the personal-access, daemon, and admin
middlewares. The login rate limiter increments cache counters but does
not emit a structured event when an attacker crosses a threshold. There
is no central audit trail of:

- successful or failed logins (user, IP, user-agent, outcome),
- privilege escalations / role changes,
- PAT or daemon-token issuance / revocation,
- file-manager operations on game servers,
- admin operations (game/node/user mutations).

**Impact**: post-incident forensics are infeasible — operators cannot
answer "who did X" or "when was account Y compromised". Defenders
cannot build alerting on top of an audit stream because there isn't
one. Compliance frameworks (SOC 2, ISO 27001) reject this baseline.

**Remediation**: add an `internal/audit/` package that emits structured
`slog` records with a stable schema (event_type, actor, resource,
outcome, request_id, ip, ts). Wire it into:

1. Auth middleware rejection paths.
2. `LoginRateLimitMiddleware.recordFailure` and threshold-cross events.
3. RBAC denials in `IsAdminMiddleware`, `AbilityChecker.CheckOrError`.
4. Sensitive-op handlers: user PUT, role assignment, PAT post/delete,
   daemon token issuance, node CRUD, file delete/move/rename, plugin
   install/uninstall.

Add a per-request correlation ID middleware so logs from one HTTP
request can be joined across components.

---

### C-4 · **Medium** · Tokens accepted via `?token=` query string

| | |
| --- | --- |
| File | `internal/api/middlewares/auth.go:172-176` |
| CWE | CWE-598 (Information exposure through query strings in GET request) |
| ASVS | 8.1.1 (No sensitive data in URLs), 7.1.1 (No sensitive data in logs) |

```go
// Try to extract from query parameter (useful for WebSocket connections)
token := r.URL.Query().Get("token")
if token != "" {
    return token
}
```

Although the comment explains the WebSocket motivation, this path lets
any client put a long-lived PASETO/PAT in the URL, which then:
- ends up in upstream proxy/CDN access logs,
- appears in browser history,
- is sent in `Referer` on cross-origin navigations.

**Impact**: low chance of direct exploitation on a well-configured
deployment, but the project has *no contractual mitigation* — no
explicit Referer-Policy header (see C-2), no log scrubbing in the
panel's own access logs, no warning to operators.

**Remediation**: keep query-token support for WebSocket only and gate
it on the URL prefix (`/api/ws/...`). Reject the query token on all
other routes. Add `Referrer-Policy: strict-origin-when-cross-origin`
(C-2 ships this anyway). Document the operator obligation: do not log
query strings on reverse proxies.

---

### C-5 · **High** · `node.GdaemonAPIKey` stored as plaintext, used per-request by gRPC interceptor

| | |
| --- | --- |
| Files | `internal/domain/node.go:24` (`db:"gdaemon_api_key"`), `internal/grpc/interceptors/auth.go:177`, `internal/enrollment/service.go:105` |
| CWE | CWE-256 (Plaintext storage of credential), CWE-312 (Cleartext storage of sensitive information) |
| ASVS | 2.10.2 (Service-account secrets encrypted at rest), 6.1.3 (Sensitive data encrypted at rest), 8.3.4 (Classification) |

There are **two** daemon credentials, only one of which has been
hardened:

- `gdaemon_api_token` (`migrations/postgres/007_hash_daemon_api_tokens.go`)
  — used by the legacy HTTP `/gdaemon_api/*` endpoints — now SHA-256 in DB.
- `gdaemon_api_key` (`internal/domain/node.go:24`) — used by the new
  gRPC bidi stream (`internal/grpc/interceptors/auth.go:177`,
  `internal/grpc/gateway/service.go:306`) — **still plaintext in DB**.
  Generated at enrollment time (`internal/enrollment/service.go:105`)
  and used verbatim for every request.

The plaintext-in-DB design is forced by the current model: gRPC presents
the key, panel `secureCompare`s it against the DB row, and a one-way
hash would break the comparison. The same constraint applies to the
HTTP token *if you want clients to keep sending the plaintext after the
migration* — which is exactly why migration 007 also adds
hash-on-the-way-in to the HTTP middleware. The gRPC path has no such
hash-on-the-way-in.

**Impact**: a database read (SQL injection elsewhere, backup leak,
read-only DB compromise) discloses live daemon credentials usable to
fully impersonate any node — execute commands, exfiltrate files,
pivot to game-server hosts.

**Remediation**: mirror what migration 007 did for the HTTP token:
- generate token client-side as `clientKey`, store `SHA256(clientKey)`
  on the panel,
- gRPC interceptor hashes the presented key before `secureCompare`,
- daemon stores plaintext locally (it has to present it),
- ship a migration that rolls existing rows over and forces re-enrollment
  for daemons that did not pick up the new format.

---

### C-6 · **Medium** · PAT and daemon API tokens stored as raw SHA-256 (GPU-friendly)

| | |
| --- | --- |
| Files | `internal/api/middlewares/auth.go:255`, `internal/api/middlewares/daemon.go:44`, `pkg/strings/sha256.go` |
| CWE | CWE-916 (Use of password hash with insufficient computational effort) |
| ASVS | 2.4.x (KDF) — verbatim L2 wording targets passwords but the spirit applies to any high-entropy bearer credential whose pre-image enables impersonation |

PATs and (now-hashed) daemon API tokens use raw SHA-256. The secret
itself is high-entropy (48 random bytes / 64 chars), which mitigates
the practical attack — exhaustive search remains infeasible — but the
storage choice is still below modern best practice:

- if an attacker dumps the token table, they get *constant-time*
  ability to test guesses (no work factor),
- if anyone ever introduces a shorter token format or a customer-chosen
  PAT, the lack of a KDF turns into an exploitable weakness immediately.

**Impact**: low under current generation parameters (48+ bytes of
`crypto/rand` entropy), but the design has zero defense-in-depth.

**Remediation**: swap SHA-256 for a memory-hard KDF (Argon2id, scrypt,
or even bcrypt) on the storage path. Token validation cost goes from
microseconds to ~10 ms per request — acceptable since the auth
middleware already does a DB read on every PAT request. Backwards
compatibility: same trick as migration 007 — accept either form during
a deprecation window, then drop the old format.

---

### C-7 · **Medium** · Setup keys are not invalidated after successful use

| | |
| --- | --- |
| File | `internal/enrollment/setup_key.go` |
| CWE | CWE-308 (Use of single-factor authentication), CWE-294 (Authentication bypass by capture-replay) |
| ASVS | 11.1.1 (Sequence of business steps), 2.5.5 (Single-use tokens), 2.3.2 (Enrollment binding) |

Setup keys are used to enroll a new daemon node. The current
implementation validates them with `subtle.ConstantTimeCompare`
(good) and expires them after 1 hour (good), but does **not** mark a
key as consumed after a successful enrollment. The same key value
remains valid for any future caller until the timeout fires.

**Impact**: if a setup key is intercepted (logged, shared in chat,
captured on the wire before the panel got TLS) it can be reused by
an attacker to enroll a rogue daemon — which then receives all of
the future task dispatch for that node and can return arbitrary
server status / file contents / console output to the panel.

**Remediation**: after the first successful `enroll` request, either
delete the setup-key row or flip a `used_at` column. Cover with a
test that asserts the second enrollment attempt with the same key
returns 401.

---

### C-8 · **Medium** · No file-upload content validation beyond filename

| | |
| --- | --- |
| Files | `internal/api/filemanager/upload/handler.go:114-151,214`, `internal/api/filemanager/filemanagerpath/path.go` |
| CWE | CWE-434 (Unrestricted file upload), CWE-646 (Reliance on file name) |
| ASVS | 12.4.1 (Type/signature validation), 12.4.2 (Content inspected for malware), 14.4.7 (`X-Content-Type-Options`) |

Upload validation checks: max size (100 MB hard cap, ✅), filename
form (no traversal, no separators, ✅), filename extension allow-list
(implicit / no MIME). It does not check:

- declared `Content-Type` vs actual magic bytes,
- whether the file looks like a known dangerous container (e.g., HTML
  with embedded scripts, polyglot images, ZIP slip),
- AV scanning hook (this is an L2 wishlist for files served back to
  browsers).

**Impact**: an uploaded HTML file with a benign-looking name (e.g.,
`logo.png` actually containing `<script>`) can be served back via the
file manager and executed in the user's session if no
`X-Content-Type-Options: nosniff` is set (see C-2). Combined with the
header gap this is exploitable; with `nosniff` it is mitigated for
browsers but the underlying validation gap remains.

**Remediation**: add `net/http.DetectContentType(buf[:512])` after the
first 512-byte read, compare against an allow-list per game (most game
servers only need configuration files, archives, save files). Add a
hook interface so deployments can plug in ClamAV or an external
scanner.

---

### C-9 · **Medium** · No MFA for any user, including admins

| | |
| --- | --- |
| Files | (none — feature absent) |
| CWE | CWE-308 (Single-factor) |
| ASVS | 2.7.x, 2.8.x, 4.3.1 (Admin MFA) |

Authentication is a single password against bcrypt + optional PAT.
There is no TOTP, no WebAuthn, no out-of-band push, no SMS, no email
code. An admin account in particular has the keys to every node and
every game server.

**Impact**: any successful credential compromise (phishing, password
reuse, leaked DB hash + offline crack) results in immediate full
control. The login rate-limiter (`login_ratelimit.go`) covers online
brute-force but not offline-cracked-hash + clean reuse.

**Remediation**: add TOTP (HOTP RFC-6238) as the first MFA factor —
relatively cheap, no third-party dependency, well-supported in
Authenticator apps. Wire it into the login flow after password
verification (`internal/api/auth/login/handler.go`) and add a
`require_mfa_for_admins` config flag. WebAuthn is the better long-term
target but TOTP unblocks the L2 requirement.

---

### C-10 · **Low** · No explicit TLS cipher-suite policy

| | |
| --- | --- |
| Files | `internal/application/application.go:281` (HTTP), `internal/application/container.go:1995,2114` (gRPC), `internal/daemon/conn.go:97` (outbound) |
| CWE | CWE-327 (Use of broken or risky cryptographic algorithm) |
| ASVS | 9.1.2 (Strong TLS configuration) |

`tls.Config` is constructed with `MinVersion: tls.VersionTLS12` on the
inbound listeners (✅) but no `CipherSuites` list. Go's defaults are
reasonable, but the policy is implicit, version-coupled, and not
auditable from the config alone.

**Impact**: low. Go's selected ciphers are modern. The risk is future
drift if a Go release reintroduces a weaker suite for compatibility,
and the inability to point an auditor at a single source of truth.

**Remediation**: add an `tlsCipherSuites()` helper that returns
`[]uint16{TLS_AES_128_GCM_SHA256, TLS_AES_256_GCM_SHA384,
TLS_CHACHA20_POLY1305_SHA256, …}` and apply uniformly to HTTP, gRPC,
and outbound clients.

---

## 4. Methodology

- **Standard**: OWASP ASVS 4.0.3, requirements flagged for L2.
- **N/A treatment**: requirements that target controls unused by a
  JSON/HTTP + gRPC backend (HTML rendering, DOM XSS, browser cookie
  shipped from the API path when none is set, LDAP, XPath/XXE,
  GraphQL, SOAP) are marked ➖ N/A with a one-line justification.
- **Evidence rule**: a requirement is ✅ Met only when at least one of
  the following holds:
  1. An automated test asserts the happy-path AND a negative case.
  2. The control lives at a single, vetted enforcement point that
     covers every relevant handler (e.g., the auth middleware).
  3. The control is delivered by a well-established stdlib /
     third-party primitive (e.g., `crypto/rand`, `bcrypt`).
- **🟡 Partial**: implementation exists but lacks one of the above,
  or its enforcement is operator-config dependent.
- **❌ Not met**: no implementation.
- **Conformance score**: `Met / (Met + Partial * 0.5 + Not met)`,
  computed per chapter and aggregated. N/A items are excluded from
  the denominator.

The audit reviewed source under `internal/`, `pkg/`, `cmd/`,
`openapi/`, `migrations/`, `.github/workflows/`, `Dockerfile`,
`docker-compose.yml`, and the existing L1 document. Test names are
preserved verbatim from the existing test suite.

---

## 5. ASVS chapter conformance

### V1 Architecture, Design and Threat Modeling

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 1.1.1 | SDLC includes security | 🟡 Partial | L1 doc + `router_security_*` test suite codify it; no SDLC policy doc. |
| 1.1.2 | Threat modelling for new features | ❌ Not met | No formal threat model. Roadmap. |
| 1.1.3 | User stories include security acceptance criteria | 🟡 Partial | Security tests live alongside features but ACs not tracked. |
| 1.1.4 | Documented and justified trust boundaries | 🟡 Partial | This document §1 lists them; `docs/PROJECT_STRUCTURE.md` + `docs/gameap_architecture.svg`. |
| 1.1.5 | Definition of security components & functions | 🟡 Partial | Implicit via package layout (`internal/api/middlewares/`, `internal/rbac/`, `pkg/auth/`). |
| 1.1.6 | Centralized, simple, vetted controls | ✅ Met | RBAC concentrated in `internal/rbac/rbac.go`; auth via single middleware chain. |
| 1.1.7 | Secure coding checklist + tooling | 🟡 Partial | `golangci-lint` (`.golangci.yaml`) + project conventions in `CLAUDE.md`. No explicit security checklist. |
| 1.2.1 | App runs with low-priv OS account | ✅ Met | `Dockerfile:52-53,57` — `addgroup -g 1000 gameap`, `USER gameap`. |
| 1.2.2 | Mutual auth between components, unique account per service | 🟡 Partial | Per-node API key + mTLS (`internal/grpc/interceptors/auth.go:102-137,177`). DB/Redis still use shared secrets from env. |
| 1.2.3 | Application uses unique account | ✅ Met | `Dockerfile:52-53,57`. |
| 1.4.1 | Trusted enforcement points (gateway + server) | ✅ Met | All AuthN/AuthZ in middleware chain (`internal/api/router.go`). |
| 1.4.4 | Single vetted access-control mechanism | ✅ Met | `internal/rbac/rbac.go`; all handlers route through `base.RBAC`. |
| 1.5.1 | Trust boundaries documented | 🟡 Partial | §1; gap on data-flow diagram inside boundaries. |
| 1.5.4 | Output encoding centralized | ✅ Met | `pkg/api/responder.go::WriteJSON`. |
| 1.6.1 | Cryptographic key inventory | ❌ Not met | No formal inventory. Roadmap. |
| 1.6.2 | Defined key-management policy | ❌ Not met | Operator manages env vars / certs manually. |
| 1.6.3 | Keys protected against unauthorised access | 🟡 Partial | TLS keys via file/inline/ACME; ACME state in `FileManager` with S3 in multi-instance. |
| 1.6.4 | Clear-text key material not stored long-term | ❌ Not met | `AUTH_SECRET`/`ENCRYPTION_KEY` live in env vars indefinitely. |
| 1.7.1 | Common logging format & error handling | 🟡 Partial | `log/slog` + `pkg/api/responder.go` + `recovery.go`. No log schema. |
| 1.7.2 | Logs transmitted to remote analysis | ❌ Not met | Stdout only. Roadmap (C-3). |
| 1.8.1 | Sensitive data classified | ❌ Not met | Roadmap. |
| 1.8.2 | Personal information access policy | ❌ Not met | No defined policy. |
| 1.9.1 | Encrypts data in transit incl. internal | 🟡 Partial | TLS optional for internal Redis / DB depending on deployment. |
| 1.9.2 | TLS or strong encryption between all components | 🟡 Partial | Same. gRPC daemon mTLS supported; cache/DB up to operator. |
| 1.10.1 | Source-code version control | ✅ Met | Git repo, `.github/workflows/`. |
| 1.11.1-3 | Business-logic architecture | 🟡 Partial | Domain layer in `internal/domain/`, services in `internal/services/`. No documented state machines for critical flows. |
| 1.12.2 | File uploads stored outside web root | ✅ Met | `internal/files/local.go` (sandboxed via `os.Root`); `FILES_LOCAL_BASE_PATH` separate from served static dir. |
| 1.14.1-6 | Configuration controls | 🟡 Partial | Env-driven (`internal/config/config.go`); no SBOM or signed build artefacts. |

**V1 score: 38%** (3 Met / 4 Partial / 6 Not met of 13 L2-applicable).

---

### V2 Authentication

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 2.1.1 | Passwords ≥ 12 chars | ❌ Not met | `internal/api/auth/login/input.go:19-29` only checks non-empty. **T-6**. |
| 2.1.2 | No truncation; allow ≥ 64 chars | 🟡 Partial | bcrypt's 72-byte limit not surfaced — `pkg/auth/password_test.go:65-72` asserts 100-byte rejection, but no user-facing clamp/error. |
| 2.1.3 | Allow Unicode and spaces | 🟡 Partial | No explicit denial; not validated either. |
| 2.1.4 | Passwords may include any printable char | 🟡 Partial | Same. |
| 2.1.5 | Users can change their password | ✅ Met | `internal/api/users/putuser/handler.go` (admin-edit + self-edit paths). |
| 2.1.6 | Re-auth required when changing password | ❌ Not met | No current-password challenge in PUT-user. |
| 2.1.7 | Reject breached / common passwords | ❌ Not met | No HIBP / dictionary check. **T-6**. |
| 2.1.8 | Password-strength meter / feedback | ❌ Not met | No frontend hook. |
| 2.1.9 | No composition rules ("must contain X") | ✅ Met | None imposed. |
| 2.1.10 | No periodic rotation | ✅ Met | No forced expiry. |
| 2.1.11 | Allow paste / browser helpers | ✅ Met | No `autocomplete=off` interference. |
| 2.1.12 | Browser-stored masking | ➖ N/A | API-level; SPA responsibility. |
| 2.2.1 | Anti-automation on credential test | ✅ Met | `LoginRateLimitMiddleware` (`login_ratelimit.go`) — 20/IP, 5/username, 15 min. Tests: `TestRouterSecurity_API2_LoginBruteForceProtection`, 9 unit tests. |
| 2.2.2 | Lockout or similar | ✅ Met | Same middleware (sliding-window TTL self-recovers). |
| 2.2.3 | Notify user of significant events | ❌ Not met | No email/web notification of new login or password change. |
| 2.2.4 | Impersonation-resistant authn (anti-phishing) | ❌ Not met | Password-only; needs WebAuthn / MFA. |
| 2.2.5 | CSP / iframe brute-force resistance | ❌ Not met | Tied to security headers gap (C-2). |
| 2.2.6 | Replay resistance | ✅ Met | PASETO `nbf`/`exp` + revocation list (`pkg/auth/revocation*.go`). |
| 2.2.7 | Intent to authenticate (e.g., user click required) | ➖ N/A | API-level. |
| 2.3.1 | System-generated initial passwords random and changeable | ➖ N/A | No system-issued passwords; admins set them. |
| 2.3.2 | Enrollment binding to authenticator | 🟡 Partial | Setup key (`internal/enrollment/setup_key.go`) is single-binding but reusable until expiry (**C-7**). |
| 2.3.3 | Renewal of enrollment instructions limited | 🟡 Partial | Operator regenerates manually. |
| 2.4.1 | Passwords stored using approved KDF | ✅ Met | bcrypt (`pkg/auth/password.go:14`). |
| 2.4.2 | Salt is random and unique | ✅ Met | bcrypt salt per-hash. |
| 2.4.3 | PBKDF2 iterations ≥ project minimum | ➖ N/A | bcrypt path is used; PBKDF2 not used for passwords. |
| 2.4.4 | bcrypt work factor ≥ 13 (L2) | 🟡 Partial | Uses `bcrypt.DefaultCost = 10` (`pkg/auth/password.go:9`). **T-8**. |
| 2.4.5 | Argon2 memory factor (where used) | ➖ N/A | Argon2 used only by plugin SDK, not for user passwords. |
| 2.5.1 | Recovery does not reveal stored hash | ➖ N/A | No recovery flow yet. |
| 2.5.2 | Recovery does not send cleartext | ➖ N/A | Same. |
| 2.5.3 | Hint not user-visible | ➖ N/A | Same. |
| 2.5.4 | No default credentials | ✅ Met | Setup token random or operator-supplied; tests `TestRouterSecurity_API8_DaemonSetupTokenValidation`, `_EnrollmentSetupKeyValidation`. |
| 2.5.5 | Forgot-password tokens single-use, time-bound | ➖ N/A | No reset flow. |
| 2.5.6 | New auth establishes new session | ✅ Met | `internal/api/auth/login/handler.go` issues new PASETO each login. |
| 2.5.7 | Recovery deny enumeration | ➖ N/A | No reset flow. |
| 2.6.x | Lookup secrets (one-time codes) | ❌ Not met | Not implemented. **T-4**. |
| 2.7.x | Out-of-band authenticators | ❌ Not met | Not implemented. **T-4**. |
| 2.8.x | OTP / TOTP | ❌ Not met | **T-4 / C-9**. |
| 2.9.1 | Cryptographic key material protected | ✅ Met | PASETO key validated (`pkg/auth/paseto.go:18-39`). |
| 2.9.2 | Verifiers stored one-way | ✅ Met | bcrypt for passwords; SHA-256 + constant-time for PAT / daemon HTTP token (note: gRPC daemon API key is plaintext — see **C-5**). Tests: `TestRouterSecurity_API2_PATSecretMustBeOpaque`, `TestRouterSecurity_API2_DaemonAPITokenStoredAsHash`. |
| 2.9.3 | Challenge prevents replay | ✅ Met | PASETO single-use claims + revocation. |
| 2.10.1 | Service-account secrets ≥ 128 bits | ✅ Met | PAT 48 random bytes (`internal/api/tokens/posttoken/handler.go`); daemon HTTP token 64 chars; setup key 32 chars; PASETO 32-byte key. |
| 2.10.2 | Service-account secrets encrypted at rest | 🟡→❌ | HTTP token: SHA-256 ✅. gRPC `gdaemon_api_key`: **plaintext** ❌ (**C-5**). |
| 2.10.3 | No hardcoded passwords in source | ✅ Met | `grep -rn "password = \"" pkg/ internal/` returns no matches outside tests. |
| 2.10.4 | No default secrets in shipped configuration | ✅ Met | `AUTH_SECRET` required at boot (`config.go:51` — `required,notEmpty`). |

**V2 score: 51%** (14 Met / 5 Partial / 13 Not met / 4 N/A; L2 denominator 32).

---

### V3 Session Management

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 3.1.1 | No session ID in URL | 🟡 Partial | Documented WebSocket exception (`auth.go:172-176`, **C-4**). |
| 3.2.1 | Session token ≥ 64 bits entropy | ✅ Met | PASETO v4.local (256-bit symmetric); PAT 48 random bytes. |
| 3.2.2 | CSPRNG | ✅ Met | `crypto/rand` (`pkg/strings/random.go`). |
| 3.2.3 | Token issued only after successful auth | ✅ Met | `internal/api/auth/login/handler.go:92-108`. |
| 3.2.4 | Created using approved algorithm | ✅ Met | PASETO v4.local; JWT HS384 fallback (`pkg/auth/jwt.go`). |
| 3.3.1 | Logout invalidates session | ✅ Met | `internal/api/auth/logout/handler.go:44-77` + denylist check (`auth.go:134-153`). Tests: `TestRouterSecurity_API2_LogoutInvalidatesToken`, `_LogoutRequiresAuth`. |
| 3.3.2 | Idle session timeout | ❌ Not met | Only absolute timeout. **T-7**. |
| 3.3.3 | Absolute timeout | ✅ Met | PASETO `exp` (`pkg/auth/paseto.go:60`); 24 h default, 7 d "remember me" (`login/handler.go:19-20`). |
| 3.3.4 | Session re-binding on privilege change | 🟡 Partial | RBAC role change visible after cache TTL (`internal/rbac/`). Test: `TestRouterSecurity_API5_Escalation_RemovedAdminRoleLosesAccess`. No forced token re-issue. |
| 3.4.1 | Cookies marked Secure | 🟡 Partial | API does not issue cookies; cookie path in `auth.go:179` exists but operator-set. |
| 3.4.2 | Cookies marked HttpOnly | 🟡 Partial | Same. |
| 3.4.3 | Cookies marked SameSite | 🟡 Partial | Same. |
| 3.4.4 | `__Host-` prefix where applicable | ➖ N/A | No cookies issued by API. |
| 3.4.5 | Path attribute on cookies | ➖ N/A | Same. |
| 3.5.1 | Logout endpoint accessible from all pages | ✅ Met | `POST /api/auth/logout` (`internal/api/auth/logout/handler.go`). |
| 3.5.2 | No use after expiry | ✅ Met | `TestRouterSecurity_API2_BrokenAuthentication`. |
| 3.5.3 | Stateless tokens signed/encrypted | ✅ Met | PASETO v4.local (authenticated encryption). |
| 3.6.1 | Federation re-auth periodic | ➖ N/A | No federated identity provider. |
| 3.7.1 | Re-auth before sensitive operations | ❌ Not met | No current-password challenge on password/PAT changes. |

**V3 score: 67%** (8 Met / 4 Partial / 3 Not met / 2 N/A; L2 denominator 15).

---

### V4 Access Control

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 4.1.1 | Server-side enforcement | ✅ Met | Auth + RBAC middlewares (`internal/api/middlewares/auth.go`); `TestRouterSecurity_API5_BFLA_*`. |
| 4.1.2 | AC attributes not user-controllable | ✅ Met | `TestRouterSecurity_API3_Escalation_RegularUserCannotEditOtherUsers`, `FuzzPutUserBody_MassAssignment`. |
| 4.1.3 | Least privilege | ✅ Met | Granular abilities (`internal/domain/rbac.go`); assignment guard `internal/domain/auth.go:157-166`. |
| 4.1.4 | Deny by default | ✅ Met | Auth middleware 401 on missing token (`auth.go:86-97`); admin middleware 403 on missing ability (`auth.go:399-403`). |
| 4.1.5 | AC failures logged, alerts on repeats | 🟡→❌ | Failures surface to client but **not** to a structured audit log (**C-3 / T-2**). |
| 4.2.1 | Sensitive data and APIs protected from IDOR | ✅ Met | `serverfinder.go:30-57` + `TestRouterSecurity_API1_BOLA_*` (7 cases) + `FuzzServerIDPathParam_*`, `FuzzFileManagerPath_*`. |
| 4.2.2 | CSRF defenses for state-changing ops | 🟡 Partial | Authorization header default mitigates; cookie path lacks SameSite. |
| 4.3.1 | Admin interfaces use MFA | ❌ Not met | **T-4 / C-9**. |
| 4.3.2 | Admin functions only accessible to admins | ✅ Met | `IsAdminMiddleware` (`auth.go:375-410`); `TestRouterSecurity_API5_BFLA_*` covers 26 admin endpoints. |
| 4.3.3 | Sensitive admin step-up auth | ❌ Not met | No re-auth before delete-user, change-role, etc. |

**V4 score: 64%** (6 Met / 2 Partial / 3 Not met of 11 L2-applicable).

---

### V5 Validation, Sanitization and Encoding

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 5.1.1 | Input validation on trusted layer | ✅ Met | `pkg/api/reader.go::InputReader/QueryReader` + per-handler `Validate()`. |
| 5.1.2 | HTTP parameter pollution defence | ✅ Met | `reader.go:52-60` reads first value; `net/http` rejects path-param dupes. |
| 5.1.3 | All inputs validated (type, length, range, allow-list) | ✅ Met | Per-handler; sort allow-lists at `internal/filters/order.go::ParseUserSort` + `internal/api/servers/getservers/input.go:14-21`. |
| 5.1.4 | Structured data validated against schema | 🟡 Partial | OpenAPI spec authoritative but no runtime router-level validation. |
| 5.1.5 | URL redirects validated | ➖ N/A | No user-supplied redirect targets. |
| 5.2.1 | Untrusted HTML sanitized | ➖ N/A | JSON API only. |
| 5.2.5 | Markdown/template safety | ➖ N/A | No server-side templates rendered for API responses. |
| 5.2.8 | Server-side request forgery defences | ✅ Met | Outbound URLs are config-derived (`internal/services/globalapi.go`, `internal/services/pluginstore/service.go`). |
| 5.3.1 | Output encoding contextual | ✅ Met | `encoding/json` defaults (`pkg/api/responder.go:96-102`). |
| 5.3.3 | Output encoding for SQL | ✅ Met | Squirrel placeholders (`internal/repositories/mysql/*.go`, `postgres/*.go`). |
| 5.3.4 | Parameterised queries (no string concat) | ✅ Met | Same. |
| 5.3.5 | Dynamic SQL identifiers protected | ✅ Met | `filters.ParseUserSort` returns `ErrInvalidSortField`; covered by `order_test.go::TestParseUserSort` with SQLi payloads. |
| 5.3.6 | LDAP queries protected | ➖ N/A | No LDAP. |
| 5.3.7 | OS command construction protected | ✅ Met | No `exec.Command` with user input found in the codebase. |
| 5.3.8 | XML/XPath/XXE | ➖ N/A | No XML parsing. |
| 5.3.10 | Path traversal protection | ✅ Met | `internal/api/filemanager/filemanagerpath/path.go::ValidatePath/ValidateFilename`; fuzz `FuzzFileManagerPath_*`; project uses `os.Root`. |
| 5.4.x | Memory/buffer safety | ✅ Met | Go memory safety; no `unsafe` usage in security-critical paths. |
| 5.5.1 | Serialization untrusted data prevented | ✅ Met | JSON only on the API; YAML (`goccy/go-yaml`) for trusted exports / pelican-egg manifests. |
| 5.5.2 | Insecure deserialisation libs avoided | ✅ Met | Standard `encoding/json`. |
| 5.5.4 | Safe JSON / YAML parsers | ✅ Met | `encoding/json` + `goccy/go-yaml`. |

**V5 score: 88%** (13 Met / 2 Partial / 1 Not met / 6 N/A; L2 denominator 16).

---

### V6 Stored Cryptography

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 6.1.1 | Regulated data classification | ❌ Not met | No data classification doc. |
| 6.1.2 | Sensitive data sent to non-trusted user encrypted in transit | ✅ Met | TLS for HTTP/WS/gRPC; cipher policy implicit (**C-10**). |
| 6.1.3 | Sensitive data encrypted at rest | 🟡 Partial | Password hashes ✅; HTTP daemon token hash ✅; gRPC daemon API key **plaintext** in DB (**C-5 / T-5**); user PII unencrypted. |
| 6.2.1 | Crypto modules fail securely | ✅ Met | `pkg/auth/paseto.go`, `jwt.go`, `password.go` return errors on invalid input. |
| 6.2.2 | Industry-proven crypto | ✅ Met | PASETO v4.local, bcrypt, AES-GCM, ChaCha20-Poly1305, SHA-256, mTLS X.509. |
| 6.2.3 | Initialization vectors | ✅ Met | PASETO/AEAD handles IV per-message via library. |
| 6.2.4 | RNG correct usage | ✅ Met | `crypto/rand` everywhere security-critical (`pkg/strings/random.go`, `pkg/auth/*`, `internal/enrollment/setup_key.go:77-84`). |
| 6.2.5 | No insecure modes | ✅ Met | No ECB / static IV uses. |
| 6.2.6 | Authenticated encryption | ✅ Met | PASETO v4.local. |
| 6.2.7 | Side-channel awareness | ✅ Met | `subtle.ConstantTimeCompare` for all secret compares (PAT `auth.go:255`, daemon `daemon.go:44`, setup key `setup_key.go:42`, gRPC key `grpc/interceptors/auth.go:185`). |
| 6.3.1 | Random IVs/nonces | ✅ Met | Library-managed. |
| 6.3.2 | RNG seeded properly | ✅ Met | `crypto/rand` ungettable seed. |
| 6.4.1 | Key management process | 🟡 Partial | Env vars + ACME for TLS; no KMS / Vault integration. |
| 6.4.2 | Key material isolated | 🟡 Partial | TLS keys file-mode 0600 (operator); secrets in env loaded into process memory only. No memory wipe. |

**V6 score: 58%** (5 Met / 4 Partial / 3 Not met of 12 L2-applicable).

---

### V7 Error Handling and Logging

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 7.1.1 | No sensitive data in logs | 🟡 Partial | Passwords not logged; query-string tokens (**C-4**) may leak via upstream proxy access logs. |
| 7.1.2 | No credentials / secrets in logs | 🟡 Partial | Same as 7.1.1. |
| 7.1.3 | Successful and failed authn logged | ❌ Not met | Auth middleware silently rejects (**C-3 / T-2**). |
| 7.1.4 | Logs include enough context (user, IP, ts, request_id) | 🟡 Partial | gRPC interceptor logs method/peer/duration (`internal/grpc/interceptors/logging.go`); no per-request correlation ID; HTTP requests not logged at all by default. |
| 7.2.1 | Security event logging | ❌ Not met | No audit stream (**C-3 / T-2**). |
| 7.2.2 | Log forwarding to remote / SIEM | ❌ Not met | stdout/stderr only. |
| 7.3.1 | Log integrity (write-once / append-only) | ❌ Not met | Operator-controlled. |
| 7.3.3 | Logs synced to time source | 🟡 Partial | Uses `time.Now()` (system clock); operator must maintain NTP. |
| 7.4.1 | Generic error messages to clients (≥ 500) | ✅ Met | `pkg/api/responder.go:114-116`. |
| 7.4.2 | Sensitive detail only in server logs | ✅ Met | Wrapped via `errors.WithMessage`; `responder.go` strips on 5xx. |
| 7.4.3 | Recovery / last-resort handler | ✅ Met | `internal/api/middlewares/recovery.go` + `recovery_test.go`. |

**V7 score: 46%** (4 Met / 3 Partial / 5 Not met of 11 L2-applicable).

---

### V8 Data Protection

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 8.1.1 | Sensitive data not in URLs | 🟡 Partial | `?token=` for WebSocket (**C-4**). |
| 8.1.2 | Cache controls on sensitive data | ❌ Not met | No `Cache-Control` set on auth/sensitive endpoints (**C-8 / T-1**). |
| 8.1.3 | Server-side does not cache sensitive responses | ✅ Met | No HTTP response cache layer. |
| 8.1.4 | Authenticated data not in CDN caches | 🟡 Partial | Operator concern; missing `Cache-Control` makes it the operator's problem. |
| 8.1.5 | Backup procedures | ➖ N/A | Operator responsibility. |
| 8.2.1 | Browser caching of sensitive responses controlled | ❌ Not met | Same as 8.1.2. |
| 8.3.1 | Sensitive data sent in body, not URL | 🟡 Partial | Mostly yes; token query path (**C-4**). |
| 8.3.4 | Data classified for protection | ❌ Not met | Roadmap. |
| 8.3.5 | Sensitive-data access logged | ❌ Not met | Tied to **C-3**. |
| 8.3.7 | Sensitive data masked in responses | ✅ Met | `internal/api/nodes/getnode/response.go:9-43`, `getdaemonstatus/response.go` (returns `HasAPIKey` boolean). Test: `TestRouterSecurity_API3_NodeResponseDoesNotLeakDaemonSecrets`. |
| 8.3.8 | Sensitive PII tokenisation | ❌ Not met | No PII tokenisation layer. |

**V8 score: 45%** (3 Met / 3 Partial / 4 Not met / 1 N/A; L2 denominator 10).

---

### V9 Communications

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 9.1.1 | TLS for all inbound + outbound traffic | 🟡 Partial | HTTPS redirect `internal/api/middlewares/https_redirect.go`; gRPC TLS default; outbound to `globalapi.gameap.com`, `plugins.gameap.dev` over HTTPS. Operator can run plain HTTP. |
| 9.1.2 | Strong TLS configuration | 🟡 Partial | `MinVersion: TLS 1.2` on listeners (`internal/application/application.go:281`, `container.go:1995,2114`); no explicit `CipherSuites` (**C-10**). |
| 9.1.3 | TLS for authenticated connections | ✅ Met | gRPC mTLS support (`internal/grpc/interceptors/auth.go:102-137`); `GRPC_REQUIRE_MTLS` flag. |
| 9.2.1 | Outbound to other systems uses trusted TLS | ❌ Not met for legacy daemon path (**C-1**); ✅ Met for global API / plugin store (standard `http.Client`). |
| 9.2.2 | Encrypted connections to external services | ✅ Met | `internal/services/globalapi.go`, `internal/services/pluginstore/service.go` — HTTPS by default. |
| 9.2.4 | Certificate revocation checked | 🟡 Partial | Go's default revocation checking (OCSP soft-fail). Not strict. |
| 9.2.5 | Backend TLS to DB / cache | 🟡 Partial | DSN-driven; operator opt-in. |
| 9.x | HSTS header | ❌ Not met | Tied to **C-2 / T-1**. |

**V9 score: 56%** (3 Met / 3 Partial / 2 Not met of 8 L2-applicable).

---

### V10 Malicious Code

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 10.1.1 | Code analyzed for malicious code | ➖ N/A | First-party; `golangci-lint` covers static issues. |
| 10.2.x | Malicious code search | ➖ N/A | Same. |
| 10.3.1 | Auto-update via secure channel | ➖ N/A | Self-hosted manual upgrade model. |
| 10.3.2 | Integrity verification on update | 🟡 Partial | `go.sum` (`go mod download` verifies checksums); release binaries not signed. |
| 10.3.3 | Side-channel attacks prevented | ✅ Met | Constant-time compares (see V6 / V2). |
| 10.x | Dependency vulnerability scanning | 🟡 Partial | `golangci-lint` in CI; no `govulncheck` (**T-10**). |

**V10 score: 67%** (1 Met / 2 Partial / 0 Not met / 4 N/A; L2 denominator 3).

---

### V11 Business Logic

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 11.1.1 | Sequence of business steps valid | 🟡 Partial | Setup-key flow validated (`router_security_daemon_test.go::TestRouterSecurity_API8_EnrollmentSetupKeyValidation`); no one-time-use enforcement (**C-7**). |
| 11.1.2 | Business logic limits use to expected actors | ✅ Met | RBAC + per-server scoping (`internal/rbac/`, `serverfinder.go`). |
| 11.1.3 | Trustworthy time stamps | ✅ Met | `time.Now()` server-side; operator NTP. |
| 11.1.4 | Anti-automation on critical flows | 🟡 Partial | Login rate-limited (`login_ratelimit.go`); other write flows uncapped. |
| 11.1.5 | Limits per user (e.g., spending limits) | ➖ N/A | No financial flows. |
| 11.1.6 | No race conditions | 🟡 Partial | Transactions via `avito-tech/go-transaction-manager`; some critical flows (RBAC cache) eventually consistent. |
| 11.1.7 | Monitoring of unusual activity | ❌ Not met | No anomaly detection. |
| 11.1.8 | Alerts / responses to attacks | ❌ Not met | No alerting pipeline. |

**V11 score: 63%** (4 Met / 2 Partial / 2 Not met / 1 N/A; L2 denominator 8).

---

### V12 Files and Resources

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 12.1.1 | Maximum file size enforced | ✅ Met | 100 MB via `http.MaxBytesReader` (`internal/api/filemanager/upload/handler.go:114`); chunk upload bounded by `FILES_UPLOAD_MAX_CHUNKS × FILES_UPLOAD_CHUNK_SIZE`. |
| 12.1.2 | Files compressed safely | ➖ N/A | No auto-extraction. |
| 12.1.3 | Storage quotas per user | ❌ Not met | Not implemented. |
| 12.2.1 | File type allow-list | 🟡 Partial | Filename validated (`ValidateFilename`); no MIME / magic-byte (**C-8 / T-9**). |
| 12.3.1 | User-supplied path canonicalised | ✅ Met | `ValidatePath` / `ValidateFilename`; fuzz `FuzzFileManagerPath_*`. |
| 12.3.2 | Files written outside intended dir rejected | ✅ Met | Same; `os.Root` per project convention. |
| 12.3.3 | File metadata not used for AC decisions | ✅ Met | AC is RBAC-based. |
| 12.3.4 | Pre-existing files protected | ✅ Met | Filemanager respects ownership of game-server dir. |
| 12.3.5 | Uploaded files not executable | ✅ Met | Default perms 0o644 (`upload/handler.go:25`). |
| 12.3.6 | File extension validated | 🟡 Partial | Filename allow-list partial; no MIME (see 12.2.1). |
| 12.4.1 | Content type and signature validated | 🟡 Partial → ❌ | Filename only (**C-8**). |
| 12.4.2 | Content inspected for malware | ❌ Not met | No AV hook. |
| 12.5.1 | Files served from different domain or safe headers | 🟡 Partial | Operator-deployment concern; **C-2** would mitigate via `nosniff`. |
| 12.5.2 | Files served outside web root | ✅ Met | `FILES_LOCAL_BASE_PATH` separate from served static dir. |
| 12.6.1 | SSRF blocked | ✅ Met | Outbound URLs config-derived only. |

**V12 score: 67%** (9 Met / 2 Partial / 4 Not met / 1 N/A; L2 denominator 15).

---

### V13 API and Web Service

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 13.1.1 | API uses defined schema | ✅ Met | `openapi/openapi.yaml`. |
| 13.1.2 | Auth/session/AC same as web | ✅ Met | All `/api/*` enforced by the same middleware chain. |
| 13.1.3 | API consumes only declared content types | ✅ Met | JSON enforced via decoder; alternates rejected with 4xx. |
| 13.1.4 | Different processing paths per content type | ✅ Met | `/api/*` (JSON) vs `/gdaemon_api/*` (legacy daemon) vs gRPC (protobuf) — `internal/api/router.go`. |
| 13.1.5 | Non-browser implicit trust avoided | ✅ Met | All `/api/*` paths enforce auth; daemon path has its own middleware. |
| 13.2.1 | REST uses correct HTTP verbs | ✅ Met | Verb-specific handler binding in router. |
| 13.2.2 | JSON schema validation | 🟡 Partial | Per-handler `Validate()`; no automatic OpenAPI request validation. |
| 13.2.3 | REST CSRF defences (where cookies) | 🟡 Partial | Header default; cookie path lacks SameSite. |
| 13.2.4 | RESTful tokens carry minimum data | ✅ Met | PASETO subject `user:login:<login>` only. |
| 13.2.5 | REST checks `Origin` for CSRF | 🟡 Partial | CORS allow-list via `rs/cors` (`internal/api/middlewares/cors.go`); origin auto-derived to match TLS scheme (**covered in L1 audit**). |
| 13.3.x | SOAP | ➖ N/A | No SOAP. |
| 13.4.x | GraphQL | ➖ N/A | No GraphQL. |
| 13.x | WebSocket origin validation | 🟡 Partial | WebSocket relies on CORS gate; no explicit `Origin` check beyond CORS allow-list. |

**V13 score: 73%** (7 Met / 2 Partial / 2 Not met / 2 N/A; L2 denominator 11).

---

### V14 Configuration

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 14.1.1 | Build / deployment documented and repeatable | ✅ Met | `Makefile`, `Dockerfile`, `DOCKER.md`; multi-platform release workflow. |
| 14.1.2 | Compiler hardening flags | 🟡 Partial | `CGO_ENABLED=0` (`Dockerfile:35`), `-w -s` in ldflags (`Dockerfile:36`); no PIE/RELRO/FORTIFY (CGO disabled limits relevance). |
| 14.1.3 | Dependencies up to date and patched | 🟡 Partial | `go.mod` pinned; no Renovate / Dependabot config; no `govulncheck` (**T-10**). |
| 14.1.4 | Components inventoried | 🟡 Partial | Implicit via `go.mod`/`go.sum`; no SBOM. |
| 14.1.5 | Software Bill of Materials | ❌ Not met | Not generated. |
| 14.2.1 | Components inventoried (runtime) | 🟡 Partial | Same as 14.1.4. |
| 14.2.2 | Components not known-vulnerable | 🟡 Partial | Manually verified; no CI check. |
| 14.2.3 | Dependencies signed / verified | ✅ Met | Go module checksum DB. |
| 14.2.4 | No unmaintained components | 🟡 Partial | Operator audit. |
| 14.2.5 | Default credentials removed | ✅ Met | Setup keys random or operator-supplied; tests `TestRouterSecurity_API8_DaemonSetupTokenValidation`, `_EnrollmentSetupKeyValidation`. |
| 14.2.6 | Vulnerability disclosure programme | ❌ Not met | No `SECURITY.md`. |
| 14.3.1 | Debug disabled in production | 🟡 Partial | `LOGGER_LEVEL`, `GRPC_ENABLE_REFLECTION` flags operator-controlled; no production-mode auto-check. |
| 14.3.2 | Security HTTP headers (HSTS, etc.) | ❌ Not met | **C-2 / T-1**. |
| 14.3.3 | Cross-Origin policies (COOP/COEP, Referrer-Policy) | ❌ Not met | Same. |
| 14.4.1 | Every response has Content-Type | ✅ Met | `pkg/api/responder.go:97`. |
| 14.4.2 | Charset specified | ✅ Met | `encoding/json` default UTF-8. |
| 14.4.3 | Content-Type allow-list | ✅ Met | Handlers reject unknown types. |
| 14.4.4 | CORS scoped to trusted domains | ✅ Met | `internal/api/middlewares/cors.go::deriveDefaultOrigin` + `HTTP_ALLOWED_ORIGINS` override. Tests: `TestNewCORSMiddleware_HTTPSWhenForceHTTPS`, `_RejectsHTTPOriginWhenForceHTTPS`, `_ExplicitAllowedOriginsWinsOverAutoDerived`. |
| 14.4.5 | HTTP methods restricted | ✅ Met | Verb routing. |
| 14.4.6 | Anti-clickjacking | ❌ Not met | **C-2**. |
| 14.4.7 | `X-Content-Type-Options: nosniff` | ❌ Not met | **C-2**. |
| 14.5.1 | Server rejects unused methods | ✅ Met | Router matches verb explicitly. |
| 14.5.2 | Domain-name validation on URL construction | ✅ Met | Outbound URLs from config. |
| 14.5.3 | CORS Origin verified server-side | ✅ Met | `rs/cors`. |
| 14.5.4 | HTTP headers stripped from upstream / unsafe | 🟡 Partial | Reverse-proxy concern; no explicit strip middleware. |

**V14 score: 55%** (8 Met / 5 Partial / 6 Not met of 22 L2-applicable).

---

## 6. Test catalogue (evidence inventory)

### Standard tests (`internal/api/router_security_*_test.go`)

| File | Category | Standard | Fuzz |
| --- | --- | ---:| ---:|
| `router_security_idor_test.go` | API1 BOLA / IDOR | 7 | — |
| `router_security_idor_fuzz_test.go` | API1 BOLA / IDOR | — | 3 |
| `router_security_auth_test.go` | API2 Broken Auth | 8+ | — |
| `router_security_auth_fuzz_test.go` | API2 Broken Auth | — | 3 |
| `router_security_daemon_test.go` | API2 + API8 | 7+ | — |
| `router_security_escalation_test.go` | API3 + API5 | 10+ | — |
| `router_security_escalation_fuzz_test.go` | API3 + API5 | — | 3 |
| `router_security_test.go` | API1/API5 token + admin gating | 2 | — |
| `router_security_helpers_test.go` | shared fixtures | — | — |

### Middleware-level

- `internal/api/middlewares/auth_test.go`
- `internal/api/middlewares/personal_access_test.go`
- `internal/api/middlewares/daemon_test.go`
- `internal/api/middlewares/cors_test.go`
- `internal/api/middlewares/https_redirect_test.go`
- `internal/api/middlewares/daemon_grpc_guard_test.go`
- `internal/api/middlewares/recovery_test.go`
- `internal/api/middlewares/login_ratelimit_test.go` (9 cases)

### Library-level

- `internal/filters/order_test.go::TestParseUserSort` — 16 cases incl. `id;DROP TABLE users--`
- `pkg/auth/password_test.go`, `paseto_test.go`, `jwt_test.go`
- `internal/api/filemanager/upload/handler_test.go`
- `internal/enrollment/service_test.go`
- `internal/grpc/interceptors/auth_test.go`

### CI

- `.github/workflows/test.yaml` — unit + lint + race
- `.github/workflows/security-fuzz.yaml` — weekly Monday 03:00 UTC, 9 fuzz targets, auto-files GitHub issues on failure
- `.github/workflows/mutation-test.yaml` — mutation testing
- `.github/workflows/e2e.yaml` — end-to-end
- `.github/workflows/release.yml` — multi-platform binaries
- `.github/workflows/docker.yml` — multi-platform images

### How to reproduce

```bash
# All security regressions
go test ./internal/api/... -run '^TestRouterSecurity_'

# All middleware tests
go test ./internal/api/middlewares/... -run '^Test'

# Sort allow-list with SQL-injection payloads
go test ./internal/filters -run '^TestParseUserSort$'

# Fuzz a single target locally
go test -run NONE -fuzz=FuzzAuthMiddleware_AuthorizationHeader -fuzztime=30s ./internal/api/
```

---

## 7. Roadmap

Priority is `impact × ease`; one-line each — see §3 for context.

### Sprint 1 — high impact, low effort (~2 weeks)

1. **C-1**: raise `internal/daemon/conn.go` TLS min to 1.2, drop `InsecureSkipVerify` (1 d, add regression test).
2. **C-2 / T-1**: `SecurityHeadersMiddleware` (HSTS, X-CTO, X-Frame-Options, Referrer-Policy, CSP) + `Cache-Control: no-store` on `/api/auth/*` (1 d).
3. **C-10**: explicit `CipherSuites` in every `tls.Config` (1 d).
4. **T-10**: `govulncheck` step in `.github/workflows/test.yaml` (1 d).
5. **C-7**: mark setup keys consumed on first successful enroll (0.5 d).
6. **C-4**: limit `?token=` to WebSocket prefix only (0.5 d).

### Sprint 2 — high impact, medium effort (~2-3 weeks)

7. **C-3 / T-2**: `internal/audit/` package + correlation-ID middleware + wire into auth/AC/sensitive-op paths (5 d).
8. **C-8 / T-9**: magic-byte/MIME validation on file upload (2 d).
9. **T-6**: password policy (min length, max length surfaced, breached check via HIBP k-anonymity API) (3 d).
10. **T-7**: idle session timeout (sliding TTL in revocation cache) (3 d).
11. **T-8**: raise bcrypt cost to 13 + config knob `AUTH_BCRYPT_COST` (1 d, runtime migration via "rehash on next login").

### Sprint 3 — larger investments (~3-4 weeks)

12. **T-4 / C-9**: TOTP MFA + admin-MFA enforcement flag (10 d).
13. **T-5 / C-5**: hash `gdaemon_api_key` at rest mirroring migration 007 design (5 d, includes data migration).
14. **C-6**: migrate PAT / daemon HTTP token storage to bcrypt or scrypt (5 d, dual-form acceptance window).
15. Audit log forwarding (syslog / OTel) (3 d).
16. SBOM (CycloneDX) generation in release pipeline (2 d).

### Sprint 4 — process & docs

17. Threat-model document (1.1.2).
18. Data-classification matrix (1.8.1, 8.3.4).
19. `SECURITY.md` (14.2.6).
20. Re-auth & step-up flow for sensitive admin ops (3.7.1, 4.3.3, 2.1.6).
21. Anti-automation on write endpoints other than login (11.1.4).

After Sprints 1–3 the project should reach **~80% L2 conformance**.
Full L2 is gated on MFA (Sprint 3) and the process / governance items
in Sprint 4.

---

## 8. References

- OWASP ASVS 4.0.3 — https://owasp.org/www-project-application-security-verification-standard/
- OWASP API Security Top 10:2023 — https://owasp.org/API-Security/editions/2023/
- CWE Top 25 — https://cwe.mitre.org/top25/
- L1 baseline document — [`docs/security/ASVS.md`](./ASVS.md)
- Existing security test harness — `internal/api/router_security_*_test.go`
- Project security testing convention — every security test file must
  comment its OWASP API Top 10:2023 category in the file header and in
  each test function (project memory note).
- Migration that hashed daemon API tokens — `migrations/postgres/007_hash_daemon_api_tokens.go`
- Weekly fuzz workflow — `.github/workflows/security-fuzz.yaml`

---

## 9. Maintenance

1. When a new security test lands, append it to §6 under the relevant
   chapter.
2. When a remediation merges, update the row's status (✅/🟡/❌) and
   re-compute the chapter score.
3. Re-review on every minor release; bump **Last reviewed** in §1.
4. Critical findings (§3) must be closed before the document is
   considered authoritative for L2 certification — track them as
   GitHub issues with `security` + `asvs-l2` labels.
