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
| Primary trust boundaries | (a) public Internet → API HTTP/WS server; (b) game daemon → API gRPC bidi; (c) operator → admin endpoints; (d) plugin (WASM) → host runtime |
| L1 baseline | [`docs/security/ASVS.md`](./ASVS.md) (last reviewed 2026-05-28) |
| Last reviewed | 2026-05-28 (Sprint 1 + 2 close-out: **C-11 resolved** by `pkg/netutil/ssrf.go` + hardened `internal/plugin/hostlibrary/http.go` (custom `DialContext`, scheme allow-list, response-header allowlist, redirect re-validation, timeout cap); **C-4 resolved** by per-source token allow-list in `internal/api/middlewares/auth.go` (?token= now glst_-only; cookie now rejects PAT); **C-10 resolved** by `pkg/tlsutil/ciphers.go` applied to every TLS listener; **T-8 resolved** by `AUTH_BCRYPT_COST` (default 13) + rehash-on-login + `pkg/auth/dummy.go` timing-oracle dummy; **T-1 residual closed** — `Cache-Control: no-store` on `/api/auth/*`, `/api/profile/*`, `/api/users/*`, `/api/tokens/*` via extended `SecurityHeadersMiddleware`; **T-10 resolved** by `.github/workflows/vuln-scan.yaml` (govulncheck source + binary modes, weekly + push to main); **SECURITY.md** vulnerability-disclosure policy added; **C-8 mitigated** by `internal/api/filemanager/filemanagermime/Checker` + magic-byte sniff in upload handler; **PAT revoke** now writes a `pat:<id>` entry to the revocation denylist and the auth middleware re-checks; **re-auth helper** `internal/api/base/reauth.go` + audit events `auth.reauth.{success,failure}`; **MFA-nudge** scaffolding — config (`AUTH_REQUIRE_MFA_FOR_ADMINS` / `AUTH_MFA_HARD_FAIL_DAYS`) + `internal/services/mfanudge` + audit events `auth.mfa.nudge.{shown,snoozed}` / `auth.mfa.enrollment.{required,completed}`; **idle-timeout** scaffolding — `pkg/auth/idle_tracker.go` + config (`AUTH_SESSION_IDLE_TIMEOUT` / `AUTH_SESSION_IDLE_UPDATE_FREQ`). Prior: 2026-05-28 (C-1 / C-2 closed, C-11 opened). Prior: 2026-05-18 (C-5 / C-7 / C-9, T-4). Prior: 2026-05-16 (C-3 / T-2). |
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

Counts below are derived directly from the §5 per-chapter tables (the
2026-05-28 re-audit recomputed them after the C-1 / C-2 closures and
the new C-11 finding). Grouped requirements (e.g. `2.8.x`) count as
one.

| Chapter | Met | Partial | Not met | N/A | Chapter score |
| --- | ---:| ---:| ---:| ---:| ---:|
| V1 Architecture & threat modeling | 8 | 13 | 7 | — | 37% |
| V2 Authentication | 21 | 5 | 8 | 10 | 67% |
| V3 Session management | 9 | 5 | 2 | 3 | 67% |
| V4 Access control | 6 | 3 | 1 | — | 71% |
| V5 Validation / encoding | 13 | 2 | 0 | 5 | 93% |
| V6 Stored cryptography | 10 | 3 | 1 | — | 80% |
| V7 Error handling & logging | 6 | 3 | 2 | — | 63% |
| V8 Data protection | 2 | 4 | 4 | 1 | 25% |
| V9 Communications | 4 | 4 | 0 | — | 67% |
| V10 Malicious code | 1 | 2 | 0 | 3 | 50% |
| V11 Business logic | 3 | 2 | 2 | 1 | 50% |
| V12 Files & resources | 8 | 3 | 3 | 1 | 64% |
| V13 API & web service | 7 | 4 | 0 | 2 | 78% |
| V14 Configuration | 14 | 9 | 2 | — | 68% |
| **Total** | **112** | **62** | **32** | **26** | **64%** |

**L2 conformance: ~64%** at the 2026-05-28 re-audit baseline. The
Sprint 1 + 2 close-out commits (2026-05-28, this PR) **close C-4,
C-10, C-11, T-8, T-9, T-10 entirely and the T-1 residual** plus ship
scaffolding for T-7 / S2.4 admin MFA / S2.3 re-auth — the §5 chapter
rows have not been re-tallied in this entry but the realistic
post-close-out score lands at **~78–82%** depending on whether the
deferred residuals (admin-MFA DB+handler integration, idle-timeout
middleware wiring, full re-auth handler rollout) ship before the
next re-audit. A separate re-tally PR will refresh the table below.

The project has a strong testing baseline, solid AuthN/AuthZ
enforcement, good cryptographic defaults, a structured security
audit log (**C-3 / T-2 resolved 2026-05-16**), TOTP 2FA with
single-use bcrypt recovery codes (**C-9 resolved 2026-05-18**),
captcha-gated login, hashed `gdaemon_api_key` at rest (**C-5
resolved 2026-05-18**), single-use setup keys (**C-7 resolved
2026-05-18**), hardened safe file serving (**C-8 serving-side
mitigated 2026-05-18**), global security HTTP headers — HSTS /
CSP / X-CTO / X-Frame-Options / Referrer-Policy — (**C-2 / T-1
resolved 2026-05-28**), removal of the legacy daemon HTTP / binnapi
code path (**C-1 / T-3 resolved 2026-05-28**), the plugin WASM HTTP
SSRF defences (**C-11 resolved 2026-05-28**), per-source token
extraction policy (**C-4 resolved 2026-05-28**), explicit TLS
cipher policy (**C-10 resolved 2026-05-28**), bcrypt cost 13 + login
rehash + timing-oracle dummy (**T-8 resolved 2026-05-28**), MIME
magic-byte upload validation (**C-8 upload-side / T-9 resolved
2026-05-28**), `Cache-Control: no-store` on sensitive paths
(T-1 residual closed 2026-05-28), `govulncheck` weekly workflow
(**T-10 resolved 2026-05-28**), `SECURITY.md` VDP published, and
PAT revoke denylist + middleware re-check.

Still held back by: admin-MFA *enforcement* (scaffolding shipped,
DB+handler integration deferred — 4.3.1 stays 🟡), no idle-timeout
middleware wiring yet (scaffolding shipped, 3.3.2 stays ❌), no
SBOM / CycloneDX in release pipeline (14.1.5), no formal threat
model (1.1.2), no data classification matrix (1.8.1 / 8.3.4), and
no remote audit-log forwarding (7.2.2). Full L2 conformance now
gated on threat-model + governance deliverables (§7 Sprint 4).

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
- **TOTP two-factor auth** (RFC-6238, `pkg/twofactor/`) — secret
  AES-256-GCM-encrypted at rest with an HKDF-SHA256-derived key
  (`crypto.go`), replay-locked via a persisted last-used step
  (`totp.go:53-84`), 10 single-use **bcrypt** recovery codes
  (`recovery.go`), a scope-confined `g2fa_` challenge token whose shape
  the auth middleware refuses to exchange for a session
  (`challenge.go`), and a per-challenge 5-attempt budget
  (`twofactorverify/handler.go:26`). Tests: `pkg/twofactor/twofactor_test.go`,
  `internal/api/middlewares/auth_twofactor_security_test.go`.
- **Captcha-gated login** (`internal/services/captcha/service.go`) —
  reCAPTCHA v2/v3 + Cloudflare Turnstile, verified *before* the user
  store is touched (`login/handler.go:87-95`) so bots cannot probe
  account existence; fail-closed by default (503 on provider outage).
- **Single-use short-lived URL tokens** (`pkg/auth/shortlived.go`,
  `internal/api/auth/shorttoken/handler.go`) — `glst_` prefix, ≤10 s
  TTL, cache-deleted on first use, minting requires header auth; a safe
  alternative to putting a long-lived token in a WebSocket/download URL.
- **Safe file serving** (`internal/api/filemanager/filemanagerhttp/headers.go`)
  — inert-MIME allowlist (SVG excluded), `X-Content-Type-Options:
  nosniff`, `Content-Security-Policy: sandbox`, RFC 2231/6266
  `Content-Disposition`, so a stored HTML/SVG cannot run in the panel
  origin even though global headers (C-2) are still pending.

### 2.3 Top-10 L2 gaps (impact-ranked)

| # | Gap | ASVS req | Severity |
| --- | --- | --- | --- |
| T-1 | ~~No security HTTP headers (HSTS, X-CTO, X-Frame-Options, CSP, Referrer-Policy)~~ **Resolved 2026-05-28** — `internal/api/middlewares/security_headers.go` (commits `37f5f34` + `52edd5c`); 17 unit tests; full config block at `internal/config/config.go:180-221`. Residual `Cache-Control: no-store` on `/api/auth/*` shipped 2026-05-28 (Sprint 1+2 close-out — see §9). | 14.3.2, 14.4.6, 14.4.7, 8.1.2, 8.2.1 | ~~High~~ |
| T-2 | ~~No structured audit log (auth failures, AC denials, sensitive ops)~~ **Resolved 2026-05-16** (`internal/audit/`; remote forwarding 7.2.2 still deferred to Sprint 3) | 7.1.3, 7.2.1 | ~~High~~ |
| T-3 | ~~Legacy daemon outbound TLS allows TLS 1.0 + `InsecureSkipVerify=true`~~ **Resolved 2026-05-28** — legacy daemon code path removed entirely (commit `2ed7be2`); `internal/daemon/conn.go` + the consumer middlewares no longer exist. | 9.1.2, 9.2.1 | ~~High~~ |
| T-4 | ~~No MFA (TOTP / WebAuthn / OOB) for users or admins~~ **Resolved 2026-05-18** — TOTP 2FA + bcrypt recovery codes (C-9); admin-MFA *enforcement* scaffolding shipped 2026-05-28 — config + service + audit events; DB-migration + handler integration tracked as Sprint 3 residual. | 2.8.x, 4.3.1 | ~~High~~ |
| T-5 | ~~`node.GdaemonAPIKey` stored plaintext in DB (used per-request by gRPC)~~ **Resolved 2026-05-18** — stored `SHA256` at enrollment, gRPC interceptor hashes-then-`secureCompare` (C-5) | 2.10.2, 6.1.3 | ~~High~~ |
| T-6 | ~~No password policy (min length, breached check, max enforced)~~ **Resolved 2026-05-27** — min 12 / max 128 + no-truncation pre-hash (2026-05-20) shipped (`pkg/auth/policy.go`, `pkg/auth/password.go`); common-password blocklist (SecLists top-1M, offline; `pkg/auth/blocklist.go`, embedded asset `pkg/auth/data/passwords/common-passwords.txt.gz`) shipped 2026-05-27 with operator override `AUTH_ALLOW_WEAK_PASSWORDS` | ~~2.1.1~~, ~~2.1.7~~ | ~~Medium~~ → Done |
| T-7 | ~~No idle session timeout (only absolute 24h)~~ **Scaffolding 2026-05-28** — `pkg/auth/idle_tracker.go` (`CacheIdleTracker` + `NoopIdleTracker`) + config `AUTH_SESSION_IDLE_TIMEOUT` (default 30m) + `AUTH_SESSION_IDLE_UPDATE_FREQ` (5m). Middleware wiring + WebSocket ping integration tracked as Sprint 3 residual. 3.3.2 stays ❌ until middleware integration lands. | 3.3.2 | Medium |
| T-8 | ~~bcrypt cost stuck at `DefaultCost`=10 (L2 wants ≥13)~~ **Resolved 2026-05-28** — `pkg/auth/password.go` parameterised on `ActiveBcryptCost`; `SetDefaultBcryptCost` validates against `[MinBcryptCost=10, MaxBcryptCost=14]` and panics at boot on misconfiguration; default 13. `HashCost` exposes stored cost; `internal/api/auth/login/handler.go` rehashes-on-login (refuses downgrade). `pkg/auth/dummy.go` adds a constant-time bcrypt verify for the non-existent-user path to defeat the user-enumeration timing oracle. | 2.4.4 | ~~Medium~~ |
| T-9 | ~~No file-upload MIME / magic-byte verification~~ **Resolved 2026-05-28** — `internal/api/filemanager/filemanagermime/Checker` (default allowlist + `FILES_UPLOAD_ALLOWED_MIMES` / `FILES_UPLOAD_ALLOW_ARCHIVES` / `FILES_UPLOAD_ALLOW_BINARY`); `upload/handler.go` sniffs the first 512 bytes via `bufio.Reader.Peek` + `http.DetectContentType` before any daemon IO; rejections emit `file.upload` audit with `detected_mime` + `reason=mime_not_allowed`. 5 security tests covering HTML-as-PNG, valid PNG, plain-text config, ZIP-default-reject, ZIP-with-flag-accept. | 12.4.1, 12.4.2 | ~~Medium~~ |
| T-10 | ~~No `govulncheck` / SBOM in CI~~ **Resolved 2026-05-28** — `.github/workflows/vuln-scan.yaml` (pinned `govulncheck@v1.1.4`, source + binary modes, weekly + push to main + manual dispatch, auto-files / auto-closes a GitHub issue per failing mode, input validation on the manual-dispatch version override to defend against workflow injection). SBOM (CycloneDX) still deferred to Sprint 3. | 14.2.2, 14.2.3, 14.2.4 | ~~Medium~~ → Partial |
| T-11 (C-11) | ~~Plugin WASM HTTP host library has no SSRF guard~~ **Resolved 2026-05-28** — `pkg/netutil/ssrf.go` blocklist + rewritten `internal/plugin/hostlibrary/http.go` with custom `Transport.DialContext` (DNS-rebinding safe), `CheckRedirect` re-validation, scheme allow-list, response-header **allow**list, `TimeoutSeconds` cap, `MaxRedirects` cap, operator allow-list. Cloud-metadata IPs never bypassable. 12 SSRF-specific security tests. | 12.6.1, 5.2.8, 1.4.5 | ~~**High**~~ |

---

## 3. Critical findings

These are concrete issues uncovered during the audit that go beyond
"unimplemented L2 requirement" — each is a potential or actual security
weakness the project should address regardless of certification ambitions.
Severity uses CVSS-style reasoning weighted by realistic exploitability
in this deployment model.

---

### C-1 · ~~**High**~~ · ✅ Resolved · Legacy daemon outbound TLS path removed entirely

| | |
| --- | --- |
| File | ~~`internal/daemon/conn.go:93-99`~~ — file deleted in commit `2ed7be2` |
| CWE | CWE-295 (Improper Certificate Validation), CWE-326 (Inadequate Encryption Strength) |
| ASVS | 9.1.2 (Strong TLS configuration), 9.2.1 (Certificate validation) |
| Status | ✅ **Resolved 2026-05-28** — see "Resolution" below. |

**Resolution (2026-05-28).** Commit `2ed7be2` ("remove legacy") deleted
the entire legacy daemon code path: `internal/daemon/conn.go`,
`internal/daemon/binnapi/*`, `internal/daemon/command_legacy.go`,
`internal/api/daemonapi/*` (gettoken / getinitdata / servers / tasks)
and the consuming middlewares `internal/api/middlewares/daemon.go` +
`internal/api/middlewares/daemon_grpc_guard.go`. The `InsecureSkipVerify: true`
/ `MinVersion: tls.VersionTLS10` HTTP client no longer exists in the
codebase (verified by `grep -rln "InsecureSkipVerify" internal/ pkg/
cmd/` returning only `internal/application/multiplexer_test.go`,
which is test-only and uses a self-signed cert). The remaining
outbound clients (`internal/services/globalapi.go`,
`internal/services/pluginstore/service.go`) use stdlib defaults with
full TLS validation. The gRPC daemon transport is now the only
control-plane connection to a daemon and supports mTLS by default
(`internal/grpc/interceptors/auth.go`).

The original finding is preserved below for history.



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

**Update (2026-05-18) — unchanged.** Re-verified `internal/daemon/conn.go:96-97`:
still `InsecureSkipVerify: true` + `MinVersion: tls.VersionTLS10`.
Remains **High** (Sprint 1 item 1).

**Update (2026-05-28) — ✅ Resolved.** See "Resolution" block at the top
of this finding. The file no longer exists.

---

### C-2 · ~~**High**~~ · ✅ Resolved · Global security HTTP headers shipped

| | |
| --- | --- |
| Files | `internal/api/middlewares/security_headers.go` (316 lines) + `security_headers_test.go` (17 funcs); config block at `internal/config/config.go:180-221`; wired in `internal/application/container.go` for both HTTP and HTTPS listeners |
| CWE | CWE-693 (Protection Mechanism Failure) |
| ASVS | 14.3.2 (Security headers), 14.4.6 (Anti-clickjacking), 14.4.7 (`X-Content-Type-Options: nosniff`), 8.2.1 (`Cache-Control`) |
| Status | ✅ **Resolved 2026-05-28** — see "Resolution" below. **Residual:** `Cache-Control: no-store` on `/api/auth/*` is not yet emitted, so 8.1.2 / 8.2.1 stay ❌ (Sprint 1 follow-up). |

**Resolution (2026-05-28).** `SecurityHeadersMiddleware` ships:

- `Strict-Transport-Security: max-age=31536000` (default), emitted only on
  HTTPS requests (`r.TLS != nil`, `X-Forwarded-Proto: https`, or
  `cfg.TLS.ForceHTTPS`) so plain-HTTP dev sessions never receive HSTS;
  `SECURITY_HSTS_INCLUDE_SUBDOMAINS` / `SECURITY_HSTS_PRELOAD` /
  `SECURITY_HSTS_MAX_AGE` operator-tunable.
- `X-Content-Type-Options: nosniff` (default on).
- `X-Frame-Options: SAMEORIGIN` (default; configurable via
  `SECURITY_FRAME_OPTIONS`).
- `Referrer-Policy: strict-origin-when-cross-origin` (default;
  `SECURITY_REFERRER_POLICY`).
- `Content-Security-Policy` — generated at boot from the embedded static
  FS: `golang.org/x/net/html` tokenises `index.html` and
  `streamsaver/mitm.html` and extracts SHA-256 hashes for every inline
  `<script>` element, producing `'sha256-<base64>'` source tokens so the
  SPA can run under a strict CSP without `unsafe-inline`. The base
  policy is `default-src 'self'; base-uri 'self'; object-src 'none';
  frame-ancestors 'self'; form-action 'self'; script-src 'self' blob:
  <inline-hashes> <captcha>; style-src 'self' 'unsafe-inline';
  img-src 'self' data: blob:; font-src 'self'; connect-src 'self';
  frame-src 'self' <captcha>; worker-src 'self' blob:`. Captcha-aware:
  reCAPTCHA v2/v3 add `https://www.google.com/recaptcha/` +
  `https://www.gstatic.com/recaptcha/` (script-src) +
  `https://recaptcha.google.com/recaptcha/` (frame-src); Turnstile adds
  `https://challenges.cloudflare.com`. Operator extras:
  `SECURITY_CSP_EXTRA_SCRIPT_SRC` /
  `SECURITY_CSP_EXTRA_STYLE_SRC` / `…CONNECT_SRC` / `…IMG_SRC` /
  `…FRAME_SRC` / `…FONT_SRC`. A custom verbatim policy via
  `SECURITY_CSP_POLICY` bypasses the generator. Report-only mode via
  `SECURITY_CSP_REPORT_ONLY` and `SECURITY_CSP_REPORT_URI`.

The middleware **fails the whole boot** (`NewSecurityHeadersMiddleware`
returns an error) when CSP is enabled but the embedded static FS is
missing one of the HTML files whose inline-script hashes the policy
depends on — a corrupt build cannot ship a policy that would silently
break the SPA. The master switch `SECURITY_HEADERS_ENABLED=false`
returns `next` unwrapped so disabled deployments pay zero per-request
cost. Downstream handlers that explicitly set their own header (e.g.
the file-download path that emits `Content-Security-Policy: sandbox`
per served file) override the global value for their response.

17 test funcs in `internal/api/middlewares/security_headers_test.go`
cover: defaults, master switch off, HSTS emission on plain HTTP
(suppressed) vs `TLS`/`X-Forwarded-Proto`/`ForceHTTPS` (emitted),
HSTS formatting (`max-age=…`, `includeSubDomains`, `preload`,
`max-age=0`), CSP report-only header swap, CSP verbatim override, the
captcha provider matrix (none / reCAPTCHA v2 / reCAPTCHA v3 /
Turnstile), extra-source merging, core directives, downstream
explicit-override behaviour, embedded-FS happy path + missing-file
boot failure, inline-script hash discovery, `report-uri`, and the
HTML-tokenizer error path.

The original finding is preserved below for history.



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

**Update (2026-05-18) — still open globally.** Re-verified: no
security-headers middleware exists (`grep` over
`internal/api/middlewares/` + `router.go` returns nothing). The only
exception is the file-download path, which now sets
`X-Content-Type-Options: nosniff` + `Content-Security-Policy: sandbox`
per served file via `SafeContentHeaders` (see C-8). All JSON API, SPA
and WebSocket responses are still header-less, so the finding remains
**High** (Sprint 1 item 2).

**Update (2026-05-28) — ✅ Resolved.** See "Resolution" block at the
top of this finding. `Cache-Control: no-store` on `/api/auth/*` is the
only piece of this remediation that did NOT ship — kept open in
Sprint 1 (8.1.2 / 8.2.1 still ❌).

---

### C-3 · ~~**High**~~ · ✅ Resolved · No structured audit log for security events

| | |
| --- | --- |
| Files | `internal/api/middlewares/auth.go` (no `slog` on rejection branches), `internal/api/middlewares/personal_access.go`, ~~`internal/api/middlewares/daemon.go`~~ (file deleted in commit `2ed7be2`) |
| CWE | CWE-778 (Insufficient logging), CWE-223 (Omission of security-relevant information) |
| ASVS | 7.1.3 (Successful/failed auth logged), 7.1.4 (Sufficient context), 7.2.1 (Security event logging), 7.2.2 (Log forwarding), 4.1.5 (AC failure logging) |
| Status | ✅ **Resolved 2026-05-16** — see "Resolution" below. Residual: 7.2.2 (remote/SIEM forwarding) and 7.3.1 (log integrity) remain open (Sprint 3). |

**Resolution (2026-05-16).** Added the `internal/audit/` package
(stable schema: `event_type, category, outcome, actor, resource,
action, reason, request_id, ip, user_agent, http_method, path`; emitted
via `slog` tagged `component=audit`, failures/denials at WARN, success
at INFO; `NopLogger` when `AUDIT_ENABLED=false`). A global
`RequestContextMiddleware` (wired once in
`internal/application/container.go` over both HTTP/HTTPS servers)
assigns/propagates a sanitized `X-Request-Id` and captures IP/UA so
every record is joinable. Wired into: (1) `AuthMiddleware` /
`IsAdminMiddleware` / `DaemonAuthMiddleware` rejection paths; (2)
`LoginRateLimitMiddleware` block/failure + login-success in the login
handler; (3) RBAC denials at the single choke point
`AbilityChecker.CheckOrError` (covers all ~38 call sites via a
process-wide audit sink set in `CreateRouter`); (4) 13 sensitive-op
handlers (user update / role assign, PAT create/revoke, daemon-token
issue, node create/update/delete, file delete/rename/chmod/write/upload,
plugin install/uninstall). Secrets/token values/file contents are never
logged (`reason` is a stable enum; submitted unauthenticated logins go
to `attempted_login`, never `actor_login`). Config: `AUDIT_ENABLED`
(default true), `AUDIT_CLIENT_IP_HEADER`. Tests: `internal/audit/*_test.go`
(schema/severity, actor derivation, request-id sanitization, ClientIP)
and `internal/api/router_security_auditlog_test.go` (end-to-end).

The original finding is preserved below for history.

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

### C-4 · ~~**Medium**~~ · ✅ Resolved · Tokens accepted via `?token=` query string

| | |
| --- | --- |
| Files | `internal/api/middlewares/auth.go` (`extractToken` → `(token, source)`, `sourceAllowsTokenType`); test pin in `internal/api/middlewares/auth_query_token_security_test.go` + `internal/api/router_security_auth_test.go::TestRouterSecurity_API2_TokenViaQueryAndCookie` |
| CWE | CWE-598 (Information exposure through query strings in GET request) |
| ASVS | 8.1.1 (No sensitive data in URLs), 7.1.1 (No sensitive data in logs), 3.1.1 (No session ID in URL) |
| Status | ✅ **Resolved 2026-05-28** — see "Resolution" below. |

**Resolution (2026-05-28).** `extractToken` was refactored to return a
`(token string, source tokenSource)` pair (enum: header / query /
cookie / unknown). A new `sourceAllowsTokenType` gate runs **before**
any cryptographic verification and enforces a per-source allow-list:

- **Authorization header**: accepts every recognised token type
  (header is the canonical channel).
- **`?token=` query**: accepts ONLY the short-lived `glst_` token.
  Any PASETO/PAT/JWT in the query string is rejected.
- **Cookie**: accepts session PASETOs and `glst_`, but NOT PATs (a
  PAT is an API credential for the Authorization header, not a
  browser session).

The rejection is intentionally surfaced as "missing token" (not "wrong
source") so an attacker cannot distinguish a wrong-source rejection
from a missing credential by probing. An audit event
`auth.token.rejected` with `reason=token_source_<source>_not_allowed`
is emitted on every block. Unknown token shapes still fall through to
the existing `unknown_token_type` audit path so older audit
dashboards keep parsing.

Tests:
- `internal/api/middlewares/auth_query_token_security_test.go`
  — PAT in query rejected, PAT in cookie rejected, PASETO in query
  rejected, PASETO via Authorization still works (header bypass not
  affected), explicit `TestSourceAllowsTokenType_Matrix` pinning the
  per-source × per-token-type decision table.
- `internal/api/router_security_auth_test.go::TestRouterSecurity_API2_TokenViaQueryAndCookie`
  — `paseto_in_query_string_rejected`, valid PASETO via cookie still
  works, invalid token via either channel returns 401.

The original finding is preserved below for history.



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

**Update (2026-05-18) — partially mitigated, finding remains.** A
single-use, ≤10 s short-lived token (`glst_`,
`pkg/auth/shortlived.go`, `internal/api/auth/shorttoken/handler.go`) is
now available as the *intended* credential for URL-borne contexts: it
is cache-deleted on first use, so a copy captured from a proxy log /
history / `Referer` is worthless once the connection is established.
However, `extractToken` (`internal/api/middlewares/auth.go:227-230`)
still accepts **any** token type from `?token=` — `detectTokenType`
(`auth.go:246-265`) will happily classify a long-lived PASETO/PAT
passed in the query — so the finding is downgraded in likelihood but
not closed. Closing it requires restricting the query path to the
`glst_` prefix (Sprint 1 item 6).

---

### C-5 · ~~**High**~~ · ✅ Resolved · `node.GdaemonAPIKey` stored as plaintext, used per-request by gRPC interceptor

| | |
| --- | --- |
| Files | `internal/domain/node.go:24` (`db:"gdaemon_api_key"`), `internal/grpc/interceptors/auth.go:177-188`, `internal/enrollment/service.go:107` |
| CWE | CWE-256 (Plaintext storage of credential), CWE-312 (Cleartext storage of sensitive information) |
| ASVS | 2.10.2 (Service-account secrets encrypted at rest), 6.1.3 (Sensitive data encrypted at rest), 8.3.4 (Classification) |
| Status | ✅ **Resolved 2026-05-18** — `gdaemon_api_key` is now stored as `strings.SHA256(apiKey)` at enrollment (`internal/enrollment/service.go:107`) and the gRPC interceptor hashes the presented key before the constant-time compare (`internal/grpc/interceptors/auth.go:178-188`: `secureCompare(pkgstrings.SHA256(apiKeys[0]), node.GdaemonAPIKey)` → `subtle.ConstantTimeCompare`). A DB read no longer discloses a usable credential. The same SHA-256-without-KDF observation as the HTTP token still applies and is tracked separately as **C-6** (acceptable for a 64-char machine credential, same rationale as migration 007). |

**Resolution (2026-05-18).** The plaintext-at-rest design was replaced
with the exact pattern migration 007 used for the HTTP token: the panel
stores only `SHA256(apiKey)` (`internal/enrollment/service.go:107`),
and `extractAndVerifyAPIKey` hashes the presented plaintext before
`secureCompare` (`internal/grpc/interceptors/auth.go:178-188`). The
daemon keeps the plaintext locally (it has to present it). Tests:
`internal/enrollment/service_test.go` asserts the stored value equals
`SHA256(result.APIKey)` and is not the plaintext.

The original finding is preserved below for history.

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

### C-6 · **Medium** · PAT and gRPC daemon key stored as raw SHA-256 (GPU-friendly)

| | |
| --- | --- |
| Files | `internal/api/middlewares/auth.go:313`, `internal/grpc/interceptors/auth.go:185`, `pkg/strings/sha256.go` |
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

**Update (2026-05-18) — unchanged, scope now also covers the gRPC
daemon key.** Re-verified: PAT (`auth.go:313`) and the daemon HTTP
token (`daemon.go:53`) still use raw `pkgstrings.SHA256`. The C-5 fix
intentionally used the same raw SHA-256 for `gdaemon_api_key`, so this
finding now logically covers all three machine credentials. Acceptable
in practice (all are ≥48-byte / 64-char `crypto/rand` secrets) but the
KDF best-practice gap stands at **Medium** (Sprint 3 item 14).

**Update (2026-05-28) — scope narrowed; still Medium.** PAT
(`internal/api/middlewares/auth.go:313`) still uses raw
`pkgstrings.SHA256` for storage; the `gdaemon_api_key` (gRPC) at
`internal/grpc/interceptors/auth.go:185` deliberately uses the same
raw SHA-256 (per migration 008) for the same reason. The **daemon
HTTP token** that previously lived at `internal/api/middlewares/daemon.go:53`
is no longer auth-relevant — the consuming middleware was deleted
in commit `2ed7be2` along with the rest of the legacy daemon path.
The `gdaemon_api_token` column persists in `internal/domain/node.go:25`
and the repository layer still hashes it on write, but no non-test
code reads it anymore (verified by `grep -rn "GdaemonAPIToken"
internal/api/ internal/grpc/ internal/audit/`); the column is
dormant pending a follow-up drop migration. Finding now covers PAT
+ gRPC daemon key only. KDF best-practice gap remains at **Medium**
(Sprint 3 item 14).

**Remediation**: swap SHA-256 for a memory-hard KDF (Argon2id, scrypt,
or even bcrypt) on the storage path. Token validation cost goes from
microseconds to ~10 ms per request — acceptable since the auth
middleware already does a DB read on every PAT request. Backwards
compatibility: same trick as migration 007 — accept either form during
a deprecation window, then drop the old format.

---

### C-7 · ~~**Medium**~~ · ✅ Resolved · Setup keys are not invalidated after successful use

| | |
| --- | --- |
| File | `internal/enrollment/setup_key.go:90-96`, `internal/enrollment/service.go:121` |
| CWE | CWE-308 (Use of single-factor authentication), CWE-294 (Authentication bypass by capture-replay) |
| ASVS | 11.1.1 (Sequence of business steps), 2.5.5 (Single-use tokens), 2.3.2 (Enrollment binding) |
| Status | ✅ **Resolved 2026-05-18** — `Service.Enroll` calls `setupKeyManager.Invalidate(ctx)` on the success path after the node is persisted (`internal/enrollment/service.go:121`). `Invalidate` clears the cached key (or blanks the env-sourced value) so a second enrollment with the same key fails `Validate`. The 1 h TTL remains as a backstop. Minor residual: an `Invalidate` cache error is logged at WARN and does not fail the enroll — acceptable given the short TTL backstop. |

**Resolution (2026-05-18).** `SetupKeyManager.Invalidate`
(`setup_key.go:90-96`) was added and is invoked on the first
successful enroll (`service.go:121`), so a captured key cannot be
replayed to enroll a rogue daemon after one legitimate use.

The original finding is preserved below for history.

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

### C-8 · ~~**Medium**~~ · ✅ Resolved · No file-upload content validation beyond filename

| | |
| --- | --- |
| Files | `internal/api/filemanager/filemanagermime/allowed.go` (new), `internal/api/filemanager/upload/handler.go` (sniff + audit on reject), `internal/config/config.go` (Files.Upload allowlist) |
| CWE | CWE-434 (Unrestricted file upload), CWE-646 (Reliance on file name) |
| ASVS | 12.4.1 (Type/signature validation), 12.4.2 (Content inspected for malware), 14.4.7 (`X-Content-Type-Options`) |
| Status | ✅ **Resolved 2026-05-28** for the type/signature check (12.4.1). 12.4.2 (AV-scanner hook) stays deferred — see "Resolution". |

**Resolution (2026-05-28).** Added `internal/api/filemanager/filemanagermime/Checker`,
a centralised MIME allowlist consulted on every upload. The default
list mirrors the serve-side `inlineSafeMimes` (PNG/JPEG/GIF/WebP/BMP/
PDF/text/plain) plus structured-text formats game-server operators
need (JSON, XML, CSV, YAML). Operator toggles:

- `FILES_UPLOAD_ALLOWED_MIMES` — additive operator extras.
- `FILES_UPLOAD_ALLOW_ARCHIVES` — unlocks ZIP/TAR/gzip/bzip2/7z/xz
  as one switch (off by default — a malicious archive can carry
  executables that a daemon-side extraction would run).
- `FILES_UPLOAD_ALLOW_BINARY` — unlocks `application/octet-stream`
  catch-all for deployments that accept opaque game-save blobs.

`internal/api/filemanager/upload/handler.go::processFiles` now:

1. Opens the multipart file, wraps it in `bufio.NewReaderSize(file, 512)`.
2. `Peek(512)` (non-consuming) into a sniff buffer.
3. `http.DetectContentType(sniff)` → bare MIME.
4. `Checker.Allowed(detected)` against the allowlist.
5. On reject: emits a `file.upload` audit event with the
   `detected_mime` and `reason=mime_not_allowed` in Extra; returns
   HTTP 415 Unsupported Media Type with the rejected MIME. No
   daemon IO is ever issued for a refused file.
6. On accept: hands the buffered reader (which still contains the
   peeked bytes — bufio.Reader does not consume on Peek) to
   `daemonFiles.UploadStream`, so the daemon sees the full file.

Tests:
- `internal/api/filemanager/filemanagermime/allowed_test.go` — defaults
  accept / reject matrix (HTML / SVG / JS / .exe / .sh / .php / ZIP
  all refused; PNG / JSON / CSV / PDF / text accepted),
  `AllowArchives` unlocks the archive group, `AllowBinary` unlocks
  only the catch-all (not executables), operator extras additive,
  parameter-insensitive (`text/plain; charset=utf-8` == `text/plain`),
  malformed inputs rejected.
- `internal/api/filemanager/upload/handler_test.go::TestHandler_C8_*`
  — HTML-payload-renamed-to-`logo.png` returns 415 + audit captures
  `detected_mime=text/html`; real PNG accepted; plain-text config
  accepted; ZIP refused by default; ZIP accepted when
  `AllowArchives=true`.

The original finding (12.4.1 surface) is preserved below for history.



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

**Update (2026-05-18) — serving side mitigated, upload validation
still open.** The download/serve path now applies
`SafeContentHeaders` (`internal/api/filemanager/filemanagerhttp/headers.go`):
only an inert-MIME allowlist (SVG deliberately excluded) is served
`inline`, everything else is forced to an opaque `attachment`, and
`X-Content-Type-Options: nosniff` + `Content-Security-Policy: sandbox`
are set on every served file regardless of the global-header gap (C-2).
The XSS-via-stored-file vector is therefore mitigated for browsers
(tests: `filemanagerhttp/headers_test.go`). The underlying upload-time
content/magic-byte validation is still absent, so the finding stays
open at **Medium** (Sprint 2 item 8) and 12.4.1/12.4.2 remain not-met.

---

### C-9 · ~~**Medium**~~ · ✅ Resolved · No MFA for any user, including admins

| | |
| --- | --- |
| Files | `pkg/twofactor/{totp,crypto,recovery,challenge,manager}.go`, `internal/api/auth/twofactorverify/handler.go`, `internal/api/auth/login/handler.go:139-216`, `internal/api/profile/twofactor/{setup,confirm,disable,recoverycodes}/` |
| CWE | CWE-308 (Single-factor) |
| ASVS | 2.6.x (look-up secrets ✅), 2.8.x (OTP/TOTP ✅), 4.3.1 (Admin MFA — 🟡 residual) |
| Status | ✅ **Resolved 2026-05-18** for the MFA *capability*. **Residual:** 2FA is opt-in per user and there is **no `require_mfa_for_admins` enforcement flag** — an admin can still be password-only, so ASVS 4.3.1 stays 🟡 Partial (tracked Sprint 4 item 20). |

**Resolution (2026-05-18).** TOTP (RFC-6238) was added as a second
factor. After the password check, an account with `TwoFactorEnabled`
receives a scope-confined `g2fa_` challenge token instead of a session
(`login/handler.go:139-216`); `/api/auth/2fa/verify`
(`twofactorverify/handler.go`) mints the session only after a valid
TOTP code or a single-use recovery code, with a per-challenge
5-attempt budget on top of the per-IP login limiter. The TOTP secret
is AES-256-GCM-encrypted at rest (HKDF-SHA256 key, `crypto.go`),
replay-locked via a persisted last-used step (`totp.go:53-84`), and the
10 recovery codes are bcrypt-hashed and single-use (`recovery.go`).
Enrollment/confirm/disable/regenerate live under
`internal/api/profile/twofactor/`. Tests: `pkg/twofactor/twofactor_test.go`,
`internal/api/auth/twofactorverify/handler_test.go`,
`internal/api/middlewares/auth_twofactor_security_test.go`,
`internal/api/profile/twofactor/*/handler_test.go`. WebAuthn remains
the better long-term anti-phishing target (2.2.4); admin-MFA
*enforcement* is the only remaining piece for full 4.3.1.

The original finding is preserved below for history.

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

### C-10 · ~~**Low**~~ · ✅ Resolved · No explicit TLS cipher-suite policy

| | |
| --- | --- |
| Files | `pkg/tlsutil/ciphers.go` (new — `ModernCipherSuites`, `PreferredCurves`, `HardenServerConfig`); applied at `internal/application/application.go` (HTTPS), `internal/application/container.go` (gRPC inbound + multiplexer) |
| CWE | CWE-327 (Use of broken or risky cryptographic algorithm) |
| ASVS | 9.1.2 (Strong TLS configuration) |
| Status | ✅ **Resolved 2026-05-28** — see "Resolution" below. |

**Resolution (2026-05-28).** Added `pkg/tlsutil/ciphers.go` as the
single source of truth for the project's TLS policy. The exported
`HardenServerConfig(*tls.Config) *tls.Config` is now called by every
TLS listener (HTTPS in `application.go`, gRPC in `container.go`,
multiplexer in `container.go::buildMultiplexerTLSConfig`).

Cipher policy:
- TLS 1.2 ciphers (Go honours these for handshakes that negotiate 1.2):
  ECDHE-ECDSA + ECDHE-RSA with **AEAD only** — AES-128-GCM,
  AES-256-GCM, ChaCha20-Poly1305. CBC suites (Lucky13), RC4, 3DES
  and static-RSA key exchange are all excluded.
- TLS 1.3: the stdlib pins TLS_AES_128_GCM_SHA256 /
  TLS_AES_256_GCM_SHA384 / TLS_CHACHA20_POLY1305_SHA256
  unconditionally — listing them in `CipherSuites` would be
  misleading because the stdlib ignores the field for 1.3.
- Curves: X25519 first (fastest + invalid-curve-attack resistant),
  P-256 second for legacy clients. P-384 / P-521 omitted (slower
  with no real upside on the wire).

`HardenServerConfig` only fills zero-valued fields — a caller that
explicitly sets a stricter `MinVersion` or a custom `CipherSuites`
list keeps its choice. The helper accepts nil and returns a
defaulted config so test code can write
`tlsutil.HardenServerConfig(nil)` without guards.

Tests: `pkg/tlsutil/ciphers_test.go` —
- `TestModernCipherSuites_OnlyAEAD` — every returned suite is AEAD.
- `TestModernCipherSuites_NoLegacy` — explicit denylist for RC4 /
  3DES / static-RSA / CBC.
- `TestPreferredCurves_X25519First` — order is pinned.
- `TestHardenServerConfig_AppliesDefaultsOnZeroValue` /
  `_DoesNotOverrideExplicitValues` / `_NilInput`.

The original finding is preserved below for history.



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

**Update (2026-05-18) — unchanged.** Re-verified: no `CipherSuites`
set anywhere in `internal/` or `pkg/`. Remains **Low** (Sprint 1 item 3).

**Update (2026-05-28) — unchanged.** Re-verified: `grep -rn "CipherSuites" internal/ pkg/ cmd/`
still returns nothing. All TLS configurations use `MinVersion: tls.VersionTLS12`
without an explicit suite list (`internal/application/application.go:267`,
`internal/application/container.go:2137`, `:2256`). Remains **Low**.

---

### C-11 · ~~**High**~~ · ✅ Resolved · Plugin WASM HTTP host library has no SSRF guard

| | |
| --- | --- |
| Files | `pkg/netutil/ssrf.go` (new); rewritten `internal/plugin/hostlibrary/http.go`; `internal/config/config.go` (`Plugin.HTTP` block); wired via `internal/application/container.go` (`hostlibrary.NewHTTPHostLibrary(cfg)`) |
| CWE | CWE-918 (SSRF), CWE-441 (Confused Deputy), CWE-200 (Information Exposure) |
| ASVS | 12.6.1 (SSRF blocked), 5.2.8 (SSRF defences), 1.4.5 (Sandbox boundaries) |
| Status | ✅ **Resolved 2026-05-28** — see "Resolution" below. |

**Resolution (2026-05-28).** Closed the supply-chain SSRF surface end
to end. Six defences land at once because removing any one of them
would re-open the category:

1. **IP blocklist** (`pkg/netutil/ssrf.go::IsBlockedIP` /
   `BlockReason`). Refuses loopback (`127/8`, `::1`), unspecified,
   link-local (`169.254/16`, `fe80::/10`), RFC1918 private,
   RFC4193 IPv6 ULA, multicast, CGNAT (`100.64/10`), reserved
   IPv4 (`0/8`, `240/4`, broadcast). Dedicated
   `IsCloudMetadataIP` covers AWS / GCP / Azure / DigitalOcean
   IMDS at `169.254.169.254`, AWS IPv6 `fd00:ec2::254`, Alibaba
   `100.100.100.200`. Cloud-metadata IPs report a distinct
   `cloud_metadata` reason so log analysis can spot exfiltration
   attempts.

2. **Custom `http.Transport.DialContext`**. Pre-dial flow:
   `SplitHostPort` → `LookupNetIP` → check every resolved IP
   against the blocklist → dial the chosen IP **verbatim** (not
   the hostname). Dialing the IP closes the DNS-rebinding
   window between resolution and connect; even if the attacker
   flips DNS mid-flight, we never re-resolve. If ANY resolved IP
   is blocked the request is refused entirely (rejects multi-IP
   attacks that mix a public + private answer).

3. **Scheme allow-list** (`PLUGINS_HTTP_ALLOWED_SCHEMES`, default
   `https`). Rejects `file://`, `ftp://`, `gopher://`, `data:`,
   `ldap://` pre-dial.

4. **Redirect re-validation**. `http.Client.CheckRedirect`
   re-runs `validateURL` on every hop and caps the chain at
   `PLUGINS_HTTP_MAX_REDIRECTS` (default 5). A public origin
   that issues `Location: http://10.0.0.1/secret` is refused at
   the redirect — the new dial would have caught it anyway, but
   the early refusal gives the plugin a clean error.

5. **Response-header allowlist** (not denylist). Default permits
   `Content-Type`, `Content-Length`, `Content-Encoding`,
   `Content-Language`, `Last-Modified`, `Etag`, `Cache-Control`,
   `Date`, `Location`, `Expires`, `Vary`. Everything else is
   stripped before the response is handed back to the plugin —
   `Set-Cookie`, `Authorization`, `WWW-Authenticate`,
   `Proxy-Authenticate`, `Clear-Site-Data`, `Server-Timing` will
   NEVER reach a plugin. Operator extras via
   `PLUGINS_HTTP_RESPONSE_HEADER_ALLOWLIST`. An allowlist (rather
   than a denylist) means new HTTP headers invented next year
   default to "stripped".

6. **TimeoutSeconds cap** (`PLUGINS_HTTP_MAX_TIMEOUT`,
   default 30s). A plugin asking for an hour-long timeout is
   clamped to the operator ceiling.

7. **Operator allow-list** (`PLUGINS_HTTP_ALLOWED_HOSTS`,
   comma-separated). Documented escape hatch for internal
   infrastructure (e.g. an in-VPC plugin store mirror). Bypasses
   the private-IP blocklist for matching hostnames; **NEVER**
   bypasses cloud-metadata IPs regardless of the allow-list —
   this is the layered defence pinned by
   `TestHTTPService_SSRF_AllowedHostCannotBypassMetadata`.

Tests in `internal/plugin/hostlibrary/http_ssrf_security_test.go`
(OWASP API7:2023 header comment):

- `TestHTTPService_SSRF_BlocksLoopback`
- `TestHTTPService_SSRF_BlocksRFC1918` (`10/8`, `172.16/12`,
  `192.168/16`)
- `TestHTTPService_SSRF_BlocksCloudMetadata` (must stay blocked
  even with `BlockPrivateIPs=false`)
- `TestHTTPService_SSRF_BlocksHostnameResolvingToPrivate` (fake
  resolver returns RFC1918 → request refused)
- `TestHTTPService_SSRF_RejectsBlockedScheme`
- `TestHTTPService_SSRF_BlocksRedirectIntoPrivate`
- `TestHTTPService_SSRF_TimeoutCap`
- `TestHTTPService_SSRF_AllowedHostBypassesBlocklist`
- `TestHTTPService_SSRF_AllowedHostCannotBypassMetadata`
- `TestHTTPService_SSRF_ResponseHeaderAllowlist` (Set-Cookie /
  Authorization / WWW-Authenticate explicitly stripped)
- `TestHTTPService_SSRF_ResponseHeaderAllowlistOperatorExtras`
- `TestHTTPService_SSRF_MaxRedirectsCap`

Plus the standalone blocklist tests in `pkg/netutil/ssrf_test.go`
covering every category (loopback / RFC1918 / IPv6 ULA / link-local
/ cloud-metadata / unspecified / multicast / CGNAT / reserved
IPv4 / public-IP smoke / metadata-lookalike rejection /
invalid-Addr rejection).

The original finding is preserved below for history.



The plugin runtime exposes an HTTP host library (`HTTPServiceImpl.Fetch`,
lines 31-85) that lets a loaded WASM plugin issue arbitrary outbound
HTTP requests. The URL, method, headers and body all come from the
plugin and reach `stdhttp.NewRequestWithContext` verbatim (line 48).
The implementation has:

- **No scheme allow-list.** Go's stdlib HTTP client accepts `http://`
  and `https://`; `file://` is rejected by `http.Client`, but the
  absence of an explicit allow-list is bad hygiene — a future change
  to the transport could introduce a regression.
- **No host blocklist.** There is no rejection of loopback
  (`127.0.0.0/8`, `::1`), unspecified (`0.0.0.0`, `::`), link-local
  (`169.254.0.0/16`, `fe80::/10`), private (RFC1918: `10/8`, `172.16/12`,
  `192.168/16`, plus IPv6 ULA `fc00::/7`), or cloud-metadata
  (`169.254.169.254`, `fd00:ec2::254`) targets.
- **No redirect cap or re-validation.** The stdlib default is 10
  redirect hops; each next hop is not re-validated against any
  policy, so a public DNS target can redirect into RFC1918 space.
- **No response-header redaction.** Every response header — including
  `Set-Cookie`, `Authorization`, `WWW-Authenticate`, and
  `Proxy-Authenticate` — is propagated back to the plugin (lines
  75-78), so the plugin can read cookies set by any reachable origin.
- **No upper bound on `req.TimeoutSeconds`** (lines 35-37). A
  misbehaving plugin can request a 1-hour timeout that the panel
  will honour.

**Mitigations already in place** (downgrade exploitability, not severity):

- Plugin install / update endpoints are `AdminOnly: true` on
  `/api/plugin-store/plugins/{id}/install` and `.../update`
  (`internal/api/router.go`); a non-admin cannot drop a malicious
  plugin into the runtime.
- The WASM blob is SHA-256-verified against the store-supplied
  `FileHash` at install time (`pluginstore.VerifyHash` in
  `internal/services/pluginstore/service.go:263`, called from
  `internal/api/pluginstore/installplugin/handler.go:258` and
  `internal/api/pluginstore/updateplugin/handler.go:230`).
- Response body capped at 10 MB (`maxBodySize` line 16).
- Default request timeout 30 s (line 15); plugin may raise it but
  every request still inherits the caller's context (so a request
  scoped to a 30 s admin operation cannot run for an hour in
  practice — but a background scheduler with no parent cancel can).

**Impact**: a malicious or compromised plugin pivots from the panel
process to:

- AWS IMDS (`169.254.169.254/latest/meta-data/iam/security-credentials/`),
  GCP / Azure / Alibaba metadata,
- internal Redis / PostgreSQL / management endpoints bound to
  loopback or a private VLAN,
- neighbour panel instances in a shared VPC,
- any local service the panel can reach on 127.0.0.1.

The plugin then reads the response (including any headers / cookies
it received) and exfiltrates the data via its next outbound call.
Because the upstream plugin store is an internet endpoint, this is
effectively a **supply-chain risk** gated only by the SHA-256 of
whatever the store currently serves for the requested version — a
compromise of `plugins.gameap.dev` (or of an operator's mirror)
would distribute a weaponised plugin under that same hash.

**Severity**: **High** in the supply-chain scenario (an attacker who
compromises the plugin store, or who tricks an admin into installing
a malicious plugin from an attacker-controlled mirror, gets remote
SSRF inside the panel's network). **Medium** for already-trusted
operator-built plugins.

**Remediation**:

1. **Scheme allow-list**: accept only `http://` and `https://`;
   default to https-only (operator opt-in for http).
2. **Pre-dial IP blocklist**: resolve the hostname before dialling
   and reject when any resolved IP is loopback / unspecified /
   link-local / private / cloud-metadata; apply at the initial host
   *and* at every redirect target (via a custom `http.Client.Transport`
   that overrides `DialContext`).
3. **Disable redirect following** (`Client.CheckRedirect` returns
   `ErrUseLastResponse`) or re-validate the redirect target against
   the blocklist.
4. **Cap `req.TimeoutSeconds`** to a hard maximum (e.g. 30 s).
5. **Strip dangerous response headers** before returning to the
   plugin: at minimum `Set-Cookie`, `Authorization`,
   `WWW-Authenticate`, `Proxy-Authenticate`.
6. **Operator allow-list** (`PLUGINS_HTTP_ALLOWED_HOSTS=`) for
   deployments that need outbound to a specific API.
7. **Tests** covering each blocked category (loopback, RFC1918,
   metadata IP, redirect-into-loopback, oversized timeout, header
   redaction). The existing test file
   `internal/plugin/hostlibrary/http_test.go` exercises only happy
   paths — none of the SSRF categories are currently regression-tested.

Tracked as Sprint 1 follow-up (see §7 item 6.5).

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

**V1 score: 37%** (8 Met / 13 Partial / 7 Not met of 28 L2-applicable;
counts reconciled with the rows above on 2026-05-18 — no status change).

---

### V2 Authentication

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 2.1.1 | Passwords ≥ 12 chars | ✅ Met | `pkg/auth/policy.go` (`MinPasswordLength = 12`) enforced by `auth.ValidatePassword` from every password-set entry point (`internal/api/users/postusers/input.go`, `internal/api/users/putuser/input.go`, `internal/api/profile/putprofile/input.go`). Login itself is non-empty-only — ASVS does not require length on login. Tests: `pkg/auth/policy_test.go`, handler tests in the three packages above. |
| 2.1.2 | No truncation; allow ≥ 64 chars (deny > 128) | ✅ Met | `pkg/auth/password.go` pre-hashes the password with SHA-256 + base64 before bcrypt, neutralising bcrypt's 72-byte input limit. `MaxPasswordLength = 128` (`pkg/auth/policy.go`) matches the §2.1.2 upper bound and is round-tripped by `pkg/auth/password_test.go TestHashPassword_RoundTrip_128Chars`. Legacy raw-bcrypt hashes still verify via `VerifyPassword` (signals `needsRehash`); login (`internal/api/auth/login/handler.go`) and the 2FA disable / recovery-codes handlers transparently upgrade them on next successful credential check. Migration verified by `TestHandler_ServeHTTP_LegacyHashUpgradesOnLogin` (`internal/api/auth/login/handler_test.go`). |
| 2.1.3 | Allow Unicode and spaces (no truncation) | ✅ Met | `postusers.ToDomain` no longer trims the password before hashing — the exact validated bytes feed `auth.HashPassword`. The SHA-256 pre-hash keeps the digest input identical for any UTF-8 sequence (`pkg/auth/password_test.go TestHashPassword/unicode_password`). |
| 2.1.4 | Passwords may include any printable char | 🟡 Partial | Same. |
| 2.1.5 | Users can change their password | ✅ Met | `internal/api/users/putuser/handler.go` (admin-edit + self-edit paths). |
| 2.1.6 | Re-auth required when changing password | ❌ Not met | No current-password challenge in PUT-user. |
| 2.1.7 | Reject breached / common passwords | ✅ Met | Embedded common-password blocklist (SecLists top-1M filtered to ≥12 chars, ~46 K entries / ~324 KB gzipped) loaded once at boot via `auth.LoadEmbeddedBlocklist` and consulted by `auth.ValidatePassword` (`pkg/auth/blocklist.go`, `pkg/auth/policy.go`). Asset committed in-repo; rebuild procedure documented in `pkg/auth/data/passwords/README.md`. Operator override `AUTH_ALLOW_WEAK_PASSWORDS=true` emits a startup `slog.Warn` and skips the dictionary lookup (length checks still apply). Offline-only by design — HIBP k-anonymity rejected to keep zero-egress deployments viable. Login does NOT run the check, preserving access for pre-existing weak-password accounts. Tests: `pkg/auth/blocklist_test.go`, `pkg/auth/policy_test.go`, `internal/api/router_security_password_policy_test.go`. |
| 2.1.8 | Password-strength meter / feedback | ❌ Not met | No frontend hook. |
| 2.1.9 | No composition rules ("must contain X") | ✅ Met | None imposed. |
| 2.1.10 | No periodic rotation | ✅ Met | No forced expiry. |
| 2.1.11 | Allow paste / browser helpers | ✅ Met | No `autocomplete=off` interference. |
| 2.1.12 | Browser-stored masking | ➖ N/A | API-level; SPA responsibility. |
| 2.2.1 | Anti-automation on credential test | ✅ Met | `LoginRateLimitMiddleware` (`login_ratelimit.go`) — 20/IP, 5/username, 15 min. Tests: `TestRouterSecurity_API2_LoginBruteForceProtection`, 9 unit tests. |
| 2.2.2 | Lockout or similar | ✅ Met | Same middleware (sliding-window TTL self-recovers). |
| 2.2.3 | Notify user of significant events | ❌ Not met | No email/web notification of new login or password change. |
| 2.2.4 | Impersonation-resistant authn (anti-phishing) | ❌ Not met | TOTP 2FA added (C-9) but TOTP is relay-phishable; true impersonation resistance needs WebAuthn/FIDO2. |
| 2.2.5 | CSP / iframe brute-force resistance | ❌ Not met | Tied to security headers gap (C-2). |
| 2.2.6 | Replay resistance | ✅ Met | PASETO `nbf`/`exp` + revocation list (`pkg/auth/revocation*.go`). |
| 2.2.7 | Intent to authenticate (e.g., user click required) | ➖ N/A | API-level. |
| 2.3.1 | System-generated initial passwords random and changeable | ➖ N/A | No system-issued passwords; admins set them. |
| 2.3.2 | Enrollment binding to authenticator | ✅ Met | Setup key (`internal/enrollment/setup_key.go`) is single-binding and now single-use — invalidated on first successful enroll (`service.go:121`, **C-7 resolved**). |
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
| 2.6.x | Lookup secrets (one-time codes) | ✅ Met | 10 single-use recovery codes, bcrypt-hashed, plaintext shown once (`pkg/twofactor/recovery.go`; **C-9 resolved**). Test: `pkg/twofactor/twofactor_test.go`. |
| 2.7.x | Out-of-band authenticators | ❌ Not met | No push/SMS/email OOB. Optional — TOTP (2.8.x) satisfies the MFA requirement; OOB not planned. |
| 2.8.x | OTP / TOTP | ✅ Met | RFC-6238 TOTP, AES-256-GCM secret at rest, replay-locked last-used step, per-challenge 5-attempt budget (`pkg/twofactor/totp.go`, `internal/api/auth/twofactorverify/handler.go`; **C-9 resolved**). Tests: `pkg/twofactor/twofactor_test.go`, `auth_twofactor_security_test.go`. |
| 2.9.1 | Cryptographic key material protected | ✅ Met | PASETO key validated (`pkg/auth/paseto.go:18-39`). |
| 2.9.2 | Verifiers stored one-way | ✅ Met | bcrypt for passwords + 2FA recovery codes; SHA-256 + constant-time for PAT, daemon HTTP token and (since **C-5 resolved**) the gRPC `gdaemon_api_key`; TOTP secret AES-256-GCM-encrypted at rest. Tests: `TestRouterSecurity_API2_PATSecretMustBeOpaque`, `TestRouterSecurity_API2_DaemonAPITokenStoredAsHash`. |
| 2.9.3 | Challenge prevents replay | ✅ Met | PASETO single-use claims + revocation. |
| 2.10.1 | Service-account secrets ≥ 128 bits | ✅ Met | PAT 48 random bytes (`internal/api/tokens/posttoken/handler.go`); daemon HTTP token 64 chars; setup key 32 chars; PASETO 32-byte key. |
| 2.10.2 | Service-account secrets encrypted at rest | ✅ Met | HTTP token SHA-256; gRPC `gdaemon_api_key` now SHA-256 at rest with hash-then-`secureCompare` on verify (**C-5 resolved**, `internal/enrollment/service.go:107`, `internal/grpc/interceptors/auth.go:178-188`). KDF best-practice gap tracked as **C-6**. |
| 2.10.3 | No hardcoded passwords in source | ✅ Met | `grep -rn "password = \"" pkg/ internal/` returns no matches outside tests. |
| 2.10.4 | No default secrets in shipped configuration | ✅ Met | `AUTH_SECRET` required at boot (`config.go:51` — `required,notEmpty`). |

**V2 score: 67%** (21 Met / 5 Partial / 8 Not met / 10 N/A; L2
denominator 34). Up from 51% — 2FA (C-9), single-use setup key (C-7),
hashed gRPC daemon key (C-5) closed 2.6.x/2.8.x/2.3.2/2.10.2.

---

### V3 Session Management

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 3.1.1 | No session ID in URL | 🟡 Partial | Documented WebSocket exception (`auth.go:227-230`, **C-4**); a single-use ≤10 s `glst_` token (`pkg/auth/shortlived.go`) is now the intended URL credential, but the query path still accepts long-lived tokens too. |
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

**V3 score: 67%** (9 Met / 5 Partial / 2 Not met / 3 N/A; L2
denominator 16; counts reconciled with rows on 2026-05-18 — no status change).

---

### V4 Access Control

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 4.1.1 | Server-side enforcement | ✅ Met | Auth + RBAC middlewares (`internal/api/middlewares/auth.go`); `TestRouterSecurity_API5_BFLA_*`. |
| 4.1.2 | AC attributes not user-controllable | ✅ Met | `TestRouterSecurity_API3_Escalation_RegularUserCannotEditOtherUsers`, `FuzzPutUserBody_MassAssignment`. |
| 4.1.3 | Least privilege | ✅ Met | Granular abilities (`internal/domain/rbac.go`); assignment guard `internal/domain/auth.go:157-166`. |
| 4.1.4 | Deny by default | ✅ Met | Auth middleware 401 on missing token (`auth.go:86-97`); admin middleware 403 on missing ability (`auth.go:399-403`). |
| 4.1.5 | AC failures logged, alerts on repeats | 🟡 Partial | AC failures now logged as `access.denied` audit events at the single RBAC choke point + admin gate (`internal/api/servers/base/abilitychecker.go`, `internal/api/middlewares/auth.go` `IsAdminMiddleware`); automated alerting on repeats is still an operator/SIEM concern (depends on 7.2.2). |
| 4.2.1 | Sensitive data and APIs protected from IDOR | ✅ Met | `serverfinder.go:30-57` + `TestRouterSecurity_API1_BOLA_*` (7 cases) + `FuzzServerIDPathParam_*`, `FuzzFileManagerPath_*`. |
| 4.2.2 | CSRF defenses for state-changing ops | 🟡 Partial | Authorization header default mitigates; cookie path lacks SameSite. |
| 4.3.1 | Admin interfaces use MFA | 🟡 Partial | TOTP 2FA available and enforced at login when an account enables it (**C-9 resolved**), but it is opt-in — no `require_mfa_for_admins` policy, so an admin may still be password-only. Enforcement flag tracked Sprint 4 item 20. |
| 4.3.2 | Admin functions only accessible to admins | ✅ Met | `IsAdminMiddleware` (`auth.go:375-410`); `TestRouterSecurity_API5_BFLA_*` covers 26 admin endpoints. |
| 4.3.3 | Sensitive admin step-up auth | ❌ Not met | No re-auth before delete-user, change-role, etc. |

**V4 score: 71%** (6 Met / 3 Partial / 1 Not met of 10 L2-applicable).
Up from 64% — 4.3.1 moved ❌→🟡 (MFA capability shipped, enforcement pending).

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
| 5.2.8 | Server-side request forgery defences | 🟡 Partial | Application-level outbound URLs config-derived (`internal/services/globalapi.go`, `internal/services/pluginstore/service.go`). **Plugin WASM host HTTP library (`internal/plugin/hostlibrary/http.go:48`) accepts plugin-controlled URLs verbatim — see C-11**. |
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

**V5 score: 93%** (13 Met / 2 Partial / 0 Not met / 5 N/A; L2
denominator 15). 5.2.8 moved ✅ → 🟡 on 2026-05-28 because the plugin
WASM HTTP host library (**C-11**) accepts plugin-controlled URLs
verbatim — the wider application code still keeps outbound URLs
config-derived.

---

### V6 Stored Cryptography

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 6.1.1 | Regulated data classification | ❌ Not met | No data classification doc. |
| 6.1.2 | Sensitive data sent to non-trusted user encrypted in transit | ✅ Met | TLS for HTTP/WS/gRPC; cipher policy implicit (**C-10**). |
| 6.1.3 | Sensitive data encrypted at rest | 🟡 Partial | Password & recovery-code hashes ✅; HTTP + gRPC daemon credentials SHA-256 (**C-5 resolved**); TOTP secret AES-256-GCM ✅. Residual: user PII (email etc.) stored unencrypted — keeps this Partial. |
| 6.2.1 | Crypto modules fail securely | ✅ Met | `pkg/auth/paseto.go`, `jwt.go`, `password.go` return errors on invalid input. |
| 6.2.2 | Industry-proven crypto | ✅ Met | PASETO v4.local, bcrypt, AES-GCM, ChaCha20-Poly1305, SHA-256, mTLS X.509. |
| 6.2.3 | Initialization vectors | ✅ Met | PASETO/AEAD handles IV per-message via library. |
| 6.2.4 | RNG correct usage | ✅ Met | `crypto/rand` everywhere security-critical (`pkg/strings/random.go`, `pkg/auth/*`, `internal/enrollment/setup_key.go:77-84`). |
| 6.2.5 | No insecure modes | ✅ Met | No ECB / static IV uses. |
| 6.2.6 | Authenticated encryption | ✅ Met | PASETO v4.local; TOTP secret AES-256-GCM with random per-message nonce (`pkg/twofactor/crypto.go:54-63`). |
| 6.2.7 | Side-channel awareness | ✅ Met | `subtle.ConstantTimeCompare` for all secret compares (PAT `auth.go:255`, daemon `daemon.go:44`, setup key `setup_key.go:42`, gRPC key `grpc/interceptors/auth.go:185`). |
| 6.3.1 | Random IVs/nonces | ✅ Met | Library-managed. |
| 6.3.2 | RNG seeded properly | ✅ Met | `crypto/rand` ungettable seed. |
| 6.4.1 | Key management process | 🟡 Partial | Env vars + ACME for TLS; no KMS / Vault integration. |
| 6.4.2 | Key material isolated | 🟡 Partial | TLS keys file-mode 0600 (operator); secrets in env loaded into process memory only. No memory wipe. |

**V6 score: 80%** (10 Met / 3 Partial / 1 Not met of 14 L2-applicable;
counts reconciled with rows on 2026-05-18 — the prior 58% had drifted).

---

### V7 Error Handling and Logging

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 7.1.1 | No sensitive data in logs | 🟡 Partial | Passwords not logged; query-string tokens (**C-4**) may leak via upstream proxy access logs. |
| 7.1.2 | No credentials / secrets in logs | 🟡 Partial | Same as 7.1.1. |
| 7.1.3 | Successful and failed authn logged | ✅ Met | `internal/audit/` emits `auth.login.success/failure/blocked`, `auth.token.rejected`, `auth.daemon.rejected` (`internal/api/middlewares/{auth,daemon,login_ratelimit}.go`, `internal/api/auth/login`). Tests: `internal/audit/*_test.go`, `internal/api/router_security_auditlog_test.go`. |
| 7.1.4 | Logs include enough context (user, IP, ts, request_id) | ✅ Met | `audit.RequestContextMiddleware` assigns/propagates a sanitized `X-Request-Id` and captures IP/UA/method/path; every audit record carries actor + `request_id` (`internal/audit/{middleware,logger,context}.go`, wired in `internal/application/container.go`). |
| 7.2.1 | Security event logging | ✅ Met | Structured `slog` audit stream tagged `component=audit` over auth, RBAC denials and sensitive ops (`internal/audit/`, `internal/api/servers/base/abilitychecker.go`, 13 sensitive-op handlers). |
| 7.2.2 | Log forwarding to remote / SIEM | ❌ Not met | stdout/stderr only; records are tagged `component=audit` so an operator can split/forward them, but no built-in remote/SIEM shipper (Sprint 3). |
| 7.3.1 | Log integrity (write-once / append-only) | ❌ Not met | Operator-controlled. |
| 7.3.3 | Logs synced to time source | 🟡 Partial | Uses `time.Now()` (system clock); operator must maintain NTP. |
| 7.4.1 | Generic error messages to clients (≥ 500) | ✅ Met | `pkg/api/responder.go:114-116`. |
| 7.4.2 | Sensitive detail only in server logs | ✅ Met | Wrapped via `errors.WithMessage`; `responder.go` strips on 5xx. |
| 7.4.3 | Recovery / last-resort handler | ✅ Met | `internal/api/middlewares/recovery.go` + `recovery_test.go`. |

**V7 score: 63%** (6 Met / 3 Partial / 2 Not met of 11 L2-applicable).
7.2.2 (remote forwarding) and 7.3.1 (log integrity / append-only) remain
open and are tracked for Sprint 3.

---

### V8 Data Protection

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 8.1.1 | Sensitive data not in URLs | 🟡 Partial | `?token=` for WebSocket (**C-4**); single-use ≤10 s `glst_` token now the safe URL credential, but query still accepts long-lived tokens. |
| 8.1.2 | Cache controls on sensitive data | ❌ Not met | `SecurityHeadersMiddleware` shipped 2026-05-28 (**C-2 closed**) but does NOT emit `Cache-Control` on `/api/auth/*`. Sprint 1 follow-up. |
| 8.1.3 | Server-side does not cache sensitive responses | ✅ Met | No HTTP response cache layer. |
| 8.1.4 | Authenticated data not in CDN caches | 🟡 Partial | Operator concern; missing `Cache-Control` makes it the operator's problem. |
| 8.1.5 | Backup procedures | ➖ N/A | Operator responsibility. |
| 8.2.1 | Browser caching of sensitive responses controlled | ❌ Not met | Same as 8.1.2 — `Cache-Control` on auth endpoints still missing post-C-2. |
| 8.3.1 | Sensitive data sent in body, not URL | 🟡 Partial | Mostly yes; token query path (**C-4**). |
| 8.3.4 | Data classified for protection | ❌ Not met | Roadmap. |
| 8.3.5 | Sensitive-data access logged | 🟡 Partial | Sensitive *operations* (file delete/rename/chmod/write/upload, token & admin mutations) now emit audit events (**C-3 resolved**, `internal/audit/`); blanket sensitive-data *read* logging is not comprehensive. |
| 8.3.7 | Sensitive data masked in responses | ✅ Met | `internal/api/nodes/getnode/response.go:9-43`, `getdaemonstatus/response.go` (returns `HasAPIKey` boolean). Test: `TestRouterSecurity_API3_NodeResponseDoesNotLeakDaemonSecrets`. |
| 8.3.8 | Sensitive PII tokenisation | ❌ Not met | No PII tokenisation layer. |

**V8 score: 25%** (2 Met / 4 Partial / 4 Not met / 1 N/A; L2
denominator 10). 8.3.5 moved ❌→🟡 (audit covers sensitive ops); the
drop vs the prior 45% is the scoreboard now matching the detail rows.

---

### V9 Communications

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 9.1.1 | TLS for all inbound + outbound traffic | 🟡 Partial | HTTPS redirect `internal/api/middlewares/https_redirect.go`; gRPC TLS default; outbound to `globalapi.gameap.com`, `plugins.gameap.dev` over HTTPS. Operator can run plain HTTP. |
| 9.1.2 | Strong TLS configuration | 🟡 Partial | `MinVersion: tls.VersionTLS12` on listeners (`internal/application/application.go:267`, `internal/application/container.go:2137,2256`); no explicit `CipherSuites` (**C-10**). |
| 9.1.3 | TLS for authenticated connections | ✅ Met | gRPC mTLS support (`internal/grpc/interceptors/auth.go:102-137`); `GRPC_REQUIRE_MTLS` flag. |
| 9.2.1 | Outbound to other systems uses trusted TLS | ✅ Met | Standard `http.Client` for outbound (`internal/services/globalapi.go`, `internal/services/pluginstore/service.go`); legacy daemon HTTP path with `InsecureSkipVerify` was removed 2026-05-28 (**C-1 resolved**). |
| 9.2.2 | Encrypted connections to external services | ✅ Met | `internal/services/globalapi.go`, `internal/services/pluginstore/service.go` — HTTPS by default. |
| 9.2.4 | Certificate revocation checked | 🟡 Partial | Go's default revocation checking (OCSP soft-fail). Not strict. |
| 9.2.5 | Backend TLS to DB / cache | 🟡 Partial | DSN-driven; operator opt-in. |
| 9.x | HSTS header | ✅ Met | `SecurityHeadersMiddleware` emits `Strict-Transport-Security` on HTTPS requests (default `max-age=31536000`; `SECURITY_HSTS_INCLUDE_SUBDOMAINS` / `_PRELOAD` tunable). **C-2 resolved 2026-05-28**. |

**V9 score: 67%** (4 Met / 4 Partial / 0 Not met of 8 L2-applicable).
Bumped on 2026-05-28: 9.2.1 ❌ → ✅ (legacy daemon TLS path removed,
**C-1 resolved**) and 9.x HSTS ❌ → ✅ (HSTS now emitted by
`SecurityHeadersMiddleware`, **C-2 resolved**). C-10 (no explicit
`CipherSuites`) still keeps 9.1.2 at 🟡.

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

**V10 score: 50%** (1 Met / 2 Partial / 0 Not met / 3 N/A; L2
denominator 3; counts reconciled with rows on 2026-05-18 — no status change).

---

### V11 Business Logic

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 11.1.1 | Sequence of business steps valid | ✅ Met | Setup-key flow validated (`internal/api/nodes/enrollsetup/handler_test.go`, `internal/enrollment/service_test.go`) and now one-time-use — invalidated on first successful enroll (**C-7 resolved**, `internal/enrollment/service.go:121`). |
| 11.1.2 | Business logic limits use to expected actors | ✅ Met | RBAC + per-server scoping (`internal/rbac/`, `serverfinder.go`). |
| 11.1.3 | Trustworthy time stamps | ✅ Met | `time.Now()` server-side; operator NTP. |
| 11.1.4 | Anti-automation on critical flows | 🟡 Partial | Login rate-limited (`login_ratelimit.go`) **and** captcha-gated (`internal/services/captcha/`), 2FA-verify has a per-challenge 5-attempt budget; other write flows still uncapped. |
| 11.1.5 | Limits per user (e.g., spending limits) | ➖ N/A | No financial flows. |
| 11.1.6 | No race conditions | 🟡 Partial | Transactions via `avito-tech/go-transaction-manager`; some critical flows (RBAC cache) eventually consistent. |
| 11.1.7 | Monitoring of unusual activity | ❌ Not met | No anomaly detection. |
| 11.1.8 | Alerts / responses to attacks | ❌ Not met | No alerting pipeline. |

**V11 score: 50%** (3 Met / 2 Partial / 2 Not met / 1 N/A; L2
denominator 7). 11.1.1 moved 🟡→✅ (C-7); the shift vs the prior 63% is
the scoreboard now matching the detail rows.

---

### V12 Files and Resources

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 12.1.1 | Maximum file size enforced | ✅ Met | 100 MB via `http.MaxBytesReader` (`internal/api/filemanager/upload/handler.go:114`); chunk upload bounded by `FILES_UPLOAD_MAX_CHUNKS × FILES_UPLOAD_CHUNK_SIZE`. |
| 12.1.2 | Files compressed safely | ➖ N/A | No auto-extraction. |
| 12.1.3 | Storage quotas per user | ❌ Not met | Not implemented. |
| 12.2.1 | File type allow-list | 🟡 Partial | Filename validated (`ValidateFilename`); no MIME / magic-byte (**C-8 / T-9**). |
| 12.3.1 | User-supplied path canonicalised | ✅ Met | Hardened `ValidatePath`/`ValidateFilename` (`filemanagerpath/path.go`) rejects NUL, backslash and any `..` component before the value is joined under `node.WorkPath/server.Dir`; fuzz `FuzzFileManagerPath_*`, tests `filemanagerpath/path_test.go`. |
| 12.3.2 | Files written outside intended dir rejected | ✅ Met | Same; `os.Root` per project convention. |
| 12.3.3 | File metadata not used for AC decisions | ✅ Met | AC is RBAC-based. |
| 12.3.4 | Pre-existing files protected | ✅ Met | Filemanager respects ownership of game-server dir. |
| 12.3.5 | Uploaded files not executable | ✅ Met | Default perms 0o644 (`upload/handler.go:25`). |
| 12.3.6 | File extension validated | 🟡 Partial | Filename allow-list partial; no MIME (see 12.2.1). |
| 12.4.1 | Content type and signature validated | ❌ Not met | No upload-time MIME/magic-byte check (**C-8**, Sprint 2). Serving side is mitigated separately (see 12.5.1) but upload content is still unvalidated. |
| 12.4.2 | Content inspected for malware | ❌ Not met | No AV hook. |
| 12.5.1 | Files served from different domain or safe headers | ✅ Met | `SafeContentHeaders` (`internal/api/filemanager/filemanagerhttp/headers.go`) sets `X-Content-Type-Options: nosniff` + `Content-Security-Policy: sandbox` on every served file, serves only an inert-MIME allowlist `inline` (SVG excluded) and forces everything else to an opaque `attachment` with an RFC 2231/6266 disposition. Test: `filemanagerhttp/headers_test.go`. |
| 12.5.2 | Files served outside web root | ✅ Met | `FILES_LOCAL_BASE_PATH` separate from served static dir. |
| 12.6.1 | SSRF blocked | 🟡 Partial | Application outbound URLs config-derived. **Plugin WASM HTTP host library (`internal/plugin/hostlibrary/http.go:48`) accepts plugin-controlled URLs verbatim — see C-11**. |

**V12 score: 64%** (8 Met / 3 Partial / 3 Not met / 1 N/A; L2
denominator 14). 12.5.1 moved 🟡→✅ on 2026-05-18 (safe file-serving
headers, #17); 12.4.1 stays ❌ pending upload validation (C-8);
12.6.1 moved ✅→🟡 on 2026-05-28 because the plugin WASM HTTP host
library (**C-11**) accepts plugin-controlled URLs verbatim.

---

### V13 API and Web Service

| # | Requirement | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 13.1.1 | API uses defined schema | ✅ Met | `openapi/openapi.yaml`. |
| 13.1.2 | Auth/session/AC same as web | ✅ Met | All `/api/*` enforced by the same middleware chain. |
| 13.1.3 | API consumes only declared content types | ✅ Met | JSON enforced via decoder; alternates rejected with 4xx. |
| 13.1.4 | Different processing paths per content type | ✅ Met | `/api/*` (JSON) versus gRPC (protobuf) listener with its own interceptor chain — `internal/api/router.go`, `internal/grpc/server.go`, `internal/grpc/interceptors/auth.go`. |
| 13.1.5 | Non-browser implicit trust avoided | ✅ Met | All `/api/*` paths enforce auth; daemon path has its own middleware. |
| 13.2.1 | REST uses correct HTTP verbs | ✅ Met | Verb-specific handler binding in router. |
| 13.2.2 | JSON schema validation | 🟡 Partial | Per-handler `Validate()`; no automatic OpenAPI request validation. |
| 13.2.3 | REST CSRF defences (where cookies) | 🟡 Partial | Header default; cookie path lacks SameSite. |
| 13.2.4 | RESTful tokens carry minimum data | ✅ Met | PASETO subject `user:login:<login>` only. |
| 13.2.5 | REST checks `Origin` for CSRF | 🟡 Partial | CORS allow-list via `rs/cors` (`internal/api/middlewares/cors.go`); origin auto-derived to match TLS scheme (**covered in L1 audit**). |
| 13.3.x | SOAP | ➖ N/A | No SOAP. |
| 13.4.x | GraphQL | ➖ N/A | No GraphQL. |
| 13.x | WebSocket origin validation | 🟡 Partial | WebSocket relies on CORS gate; no explicit `Origin` check beyond CORS allow-list. |

**V13 score: 78%** (7 Met / 4 Partial / 0 Not met / 2 N/A; L2
denominator 11; counts reconciled with rows on 2026-05-18 — no status change).

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
| 14.3.2 | Security HTTP headers (HSTS, etc.) | ✅ Met | Global `SecurityHeadersMiddleware` (`internal/api/middlewares/security_headers.go`) emits HSTS / X-CTO / X-Frame-Options / Referrer-Policy / CSP. Full config at `internal/config/config.go:180-221`. 17 tests. **C-2 / T-1 resolved 2026-05-28**. |
| 14.3.3 | Cross-Origin policies (COOP/COEP, Referrer-Policy) | 🟡 Partial | `Referrer-Policy` emitted by `SecurityHeadersMiddleware` (default `strict-origin-when-cross-origin`). COOP / COEP / CORP not yet emitted. |
| 14.4.1 | Every response has Content-Type | ✅ Met | `pkg/api/responder.go:97`. |
| 14.4.2 | Charset specified | ✅ Met | `encoding/json` default UTF-8. |
| 14.4.3 | Content-Type allow-list | ✅ Met | Handlers reject unknown types. |
| 14.4.4 | CORS scoped to trusted domains | ✅ Met | `internal/api/middlewares/cors.go::deriveDefaultOrigin` + `HTTP_ALLOWED_ORIGINS` override. Tests: `TestNewCORSMiddleware_HTTPSWhenForceHTTPS`, `_RejectsHTTPOriginWhenForceHTTPS`, `_ExplicitAllowedOriginsWinsOverAutoDerived`. |
| 14.4.5 | HTTP methods restricted | ✅ Met | Verb routing. |
| 14.4.6 | Anti-clickjacking | ✅ Met | `X-Frame-Options: SAMEORIGIN` (default) + CSP `frame-ancestors 'self'` via `SecurityHeadersMiddleware`. **C-2 resolved 2026-05-28**. |
| 14.4.7 | `X-Content-Type-Options: nosniff` | ✅ Met | `X-Content-Type-Options: nosniff` via `SecurityHeadersMiddleware` globally; file-download path also sets it locally (12.5.1). **C-2 resolved 2026-05-28**. |
| 14.5.1 | Server rejects unused methods | ✅ Met | Router matches verb explicitly. |
| 14.5.2 | Domain-name validation on URL construction | ✅ Met | Outbound URLs from config. |
| 14.5.3 | CORS Origin verified server-side | ✅ Met | `rs/cors`. |
| 14.5.4 | HTTP headers stripped from upstream / unsafe | 🟡 Partial | Reverse-proxy concern; no explicit strip middleware. |

**V14 score: 68%** (14 Met / 9 Partial / 2 Not met of 25 L2-applicable).
Bumped on 2026-05-28: 14.3.2 ❌ → ✅, 14.4.6 ❌ → ✅, 14.4.7 ❌ → ✅ all
flipped by `SecurityHeadersMiddleware` ship (**C-2 / T-1 resolved**);
14.3.3 ❌ → 🟡 (Referrer-Policy now emitted; COOP / COEP / CORP still
missing).

---

## 6. Test catalogue (evidence inventory)

### Standard tests (`internal/api/router_security_*_test.go`)

| File | Category | Standard | Fuzz |
| --- | --- | ---:| ---:|
| `router_security_idor_test.go` | API1 BOLA / IDOR | 7 | — |
| `router_security_idor_fuzz_test.go` | API1 BOLA / IDOR | — | 3 |
| `router_security_auth_test.go` | API2 Broken Auth | 8+ | — |
| `router_security_auth_fuzz_test.go` | API2 Broken Auth | — | 3 |
| `router_security_escalation_test.go` | API3 + API5 | 10+ | — |
| `router_security_escalation_fuzz_test.go` | API3 + API5 | — | 3 |
| `router_security_test.go` | API1/API5 token + admin gating | 2 | — |
| `router_security_password_policy_test.go` | API2 password policy (length + blocklist) | 8 | — |
| `router_security_auditlog_test.go` | API9 / API10 audit trail | 6 | — |
| `router_security_helpers_test.go` | shared fixtures | — | — |

### Middleware-level

- `internal/api/middlewares/auth_test.go`
- `internal/api/middlewares/auth_shorttoken_test.go`
- `internal/api/middlewares/auth_twofactor_security_test.go`
- `internal/api/middlewares/audit_capture_test.go`
- `internal/api/middlewares/personal_access_test.go`
- `internal/api/middlewares/cors_test.go`
- `internal/api/middlewares/https_redirect_test.go`
- `internal/api/middlewares/recovery_test.go`
- `internal/api/middlewares/login_ratelimit_test.go` (9 cases)
- `internal/api/middlewares/security_headers_test.go` (17 funcs — defaults, master switch, HSTS emission / format / max-age=0, CSP report-only, verbatim-policy override, captcha provider matrix, extra-source merging, core directives, downstream override, embedded-FS happy path + missing-file boot failure, inline-script discovery, report-URI, tokenizer error) — **added 2026-05-28, evidence for C-2 / T-1 closure**
- `internal/api/middlewares/shorttoken_scope_test.go`
- `internal/grpc/interceptors/auth_test.go` (mTLS + gRPC API-key constant-time compare; evidence for C-5 hash-at-rest)

### Library-level

- `internal/filters/order_test.go::TestParseUserSort` — 16 cases incl. `id;DROP TABLE users--`
- `pkg/auth/password_test.go`, `paseto_test.go`, `jwt_test.go`
- `internal/api/filemanager/upload/handler_test.go`
- `internal/enrollment/service_test.go` — asserts `gdaemon_api_key` stored as `SHA256`, not plaintext (C-5)
- `internal/grpc/interceptors/auth_test.go`

### 2FA / captcha / safe file serving (added 2026-05-18)

- `pkg/twofactor/twofactor_test.go` — TOTP skew/replay, AES-256-GCM round-trip, bcrypt single-use recovery codes (C-9)
- `internal/api/auth/twofactorverify/handler_test.go` — challenge-token shape rejection, 5-attempt budget, single-use consume
- `internal/api/middlewares/auth_twofactor_security_test.go` — challenge token cannot be exchanged for a session anywhere but `/api/auth/2fa/verify`
- `internal/api/profile/twofactor/{setup,confirm,disable,recoverycodes}/handler_test.go`
- `internal/api/auth/shorttoken/handler_test.go` — ≤10 s TTL, single-use, header-auth required (C-4 mitigation)
- `internal/services/captcha/service_test.go` — provider verify, v3 score threshold, fail-open/closed
- `internal/api/filemanager/filemanagerpath/path_test.go` — NUL/backslash/`..`-component rejection, `ok..ok` allowed
- `internal/api/filemanager/filemanagerhttp/headers_test.go` — inert-MIME allowlist, `nosniff`/CSP-`sandbox`, RFC 2231/6266 disposition (C-8 serving side)

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

1. ~~**C-1**: raise `internal/daemon/conn.go` TLS min to 1.2, drop `InsecureSkipVerify`.~~ ✅ **Done 2026-05-28** — legacy daemon code path removed in commit `2ed7be2`; the file no longer exists.
2. ~~**C-2 / T-1**: `SecurityHeadersMiddleware` (HSTS, X-CTO, X-Frame-Options, Referrer-Policy, CSP) + `Cache-Control: no-store` on `/api/auth/*`.~~ ✅ **Done 2026-05-28** — middleware in commits `37f5f34` + `52edd5c`; Sprint 1+2 close-out (2026-05-28) extends it to apply `Cache-Control: no-store, no-cache, must-revalidate, private` + `Pragma: no-cache` to every path under `SECURITY_SENSITIVE_PATH_PREFIXES` (default `/api/auth/`, `/api/profile/`, `/api/users/`, `/api/tokens/`).
3. ~~**C-10**: explicit `CipherSuites` in every `tls.Config`.~~ ✅ **Done 2026-05-28** — `pkg/tlsutil/ciphers.go::HardenServerConfig` applied at every TLS listener; AEAD-only cipher list; X25519-first curves.
4. ~~**T-10**: `govulncheck` step in CI.~~ ✅ **Done 2026-05-28** — `.github/workflows/vuln-scan.yaml` (pinned `govulncheck@v1.1.4`, source + binary modes, weekly + push to main + manual dispatch, auto-files / auto-closes a GitHub issue per failing mode). SBOM (CycloneDX) still deferred to Sprint 3.
5. ~~**C-7**: mark setup keys consumed on first successful enroll.~~ ✅ **Done 2026-05-18** (`internal/enrollment/service.go:121`).
6. ~~**C-4**: limit `?token=` to the `glst_` short-lived prefix only.~~ ✅ **Done 2026-05-28** — per-source token allow-list in `internal/api/middlewares/auth.go::sourceAllowsTokenType`; cookie also rejects PATs (API credential, not session). Tests: `auth_query_token_security_test.go` + `router_security_auth_test.go`.
7. ~~**C-11 / T-11** (NEW 2026-05-28): harden `internal/plugin/hostlibrary/http.go` against SSRF.~~ ✅ **Done 2026-05-28** — `pkg/netutil/ssrf.go` blocklist + custom `Transport.DialContext` (DNS-rebinding safe) + scheme allowlist + response-header allowlist + `CheckRedirect` re-validation + `TimeoutSeconds`/`MaxRedirects` caps + operator allow-list. Cloud-metadata IPs never bypassable. 12 SSRF-specific security tests.

### Sprint 2 — high impact, medium effort (~2-3 weeks)

8. ~~**C-3 / T-2**: `internal/audit/` package + correlation-ID middleware + wire into auth/AC/sensitive-op paths.~~ ✅ **Done 2026-05-16** (remote forwarding split out to Sprint 3).
9. ~~**C-8 / T-9**: magic-byte/MIME validation on file upload.~~ ✅ **Done 2026-05-28** — `internal/api/filemanager/filemanagermime/Checker` + `bufio.Peek` + `http.DetectContentType` in `upload/handler.go`. Operator-tunable via `FILES_UPLOAD_ALLOWED_MIMES` / `FILES_UPLOAD_ALLOW_ARCHIVES` / `FILES_UPLOAD_ALLOW_BINARY`. 5 C-8 security tests.
10. ~~**T-6**: password policy.~~ ✅ **Done 2026-05-27**.
11. ~~**T-7**: idle session timeout (sliding TTL).~~ 🟡 **Scaffolding 2026-05-28** — `pkg/auth/idle_tracker.go` (`IdleTracker` interface, `CacheIdleTracker`, `NoopIdleTracker`) + config `AUTH_SESSION_IDLE_TIMEOUT` (30m) + `AUTH_SESSION_IDLE_UPDATE_FREQ` (5m). **Residual**: middleware wiring (session-PASETO-only filter + post-credentials check + probabilistic refresh), WebSocket ping integration to keep long-lived connections alive — Sprint 3 follow-up. 3.3.2 stays ❌.
12. ~~**T-8**: raise bcrypt cost to 13 + config knob.~~ ✅ **Done 2026-05-28** — `AUTH_BCRYPT_COST` (default 13, range 10–14) + `SetDefaultBcryptCost` boot validation + `HashCost`-driven rehash-on-login (never downgrades) + `pkg/auth/dummy.go` constant-time bcrypt verify on non-existent-user path to defeat the user-enumeration timing oracle.

### Sprint 3 — larger investments (~3-4 weeks)

13. ~~**T-4 / C-9**: TOTP MFA.~~ ✅ **Done 2026-05-18**.
14. ~~**T-5 / C-5**: hash `gdaemon_api_key` at rest.~~ ✅ **Done 2026-05-18**.
15. **C-6**: migrate PAT / daemon HTTP token storage to bcrypt or scrypt (5 d, dual-form acceptance window).
16. Audit log forwarding (syslog / OTel) (3 d).
17. SBOM (CycloneDX) generation in release pipeline (2 d).
18. **S2.3 follow-up** — wire `internal/api/base/VerifyCurrentPassword` into PAT-create, PAT-revoke (with body), user-update, role-assign, user-delete handlers + rewrite the 17-case `posttoken/handler_test.go` matrix to carry `current_password` + a `Password` hash on `session.User`. Helper + audit events + unit tests shipped 2026-05-28.
19. **S2.4 follow-up** — DB migration `users.mfa_nudge_first_shown_at` + `mfa_nudge_snoozed_until` (postgres/mysql/sqlite), `UserRepository.UpdateMFANudgeShown/Snooze/Clear` methods, login-flow integration of `mfanudge.Service`, `POST /api/profile/mfa-nudge/snooze` endpoint, `GET /api/profile/me` response extension, OpenAPI update, frontend modal. Backend service + config + audit events shipped 2026-05-28; 4.3.1 stays 🟡 until integration lands.
20. **S2.5 follow-up** — wire `pkg/auth/CacheIdleTracker` into `internal/api/middlewares/auth.go` (post-credentials check + probabilistic refresh with PAT / glst_ / remember-me skip), record activity on session-issue in login handler, add WebSocket ping → RecordActivity bridge. Tracker package + tests + config shipped 2026-05-28.

### Sprint 4 — process & docs

21. ~~`SECURITY.md` (14.2.6).~~ ✅ **Done 2026-05-28** — top-level VDP with `security@gameap.com`, 72h ack / 14d assessment / 90d disclosure, severity-based fix SLAs.
22. Threat-model document (1.1.2).
23. Data-classification matrix (1.8.1, 8.3.4).
24. Anti-automation on write endpoints other than login (11.1.4) — PAT creation, 2FA enable/disable/regenerate, node enrollment, admin user-create / role-assign.

After the 2026-05-28 Sprint 1+2 close-out the project sits at an
estimated **~78–82%** L2 conformance (pending the §2.1 re-tally PR).
Closing the remaining S2.3 / S2.4 / S2.5 integration residuals
should push the figure into the low 90s, with the final percentage
gated on the Sprint 4 governance items (threat model, data
classification, anti-automation on write endpoints) and C-6
(bcrypt'ing the machine credentials).

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

### Change log

- **2026-05-28 — Sprint 1 + 2 close-out.** Closed the remaining
  open critical/medium findings from the Sprint 1 + 2 roadmap and
  shipped scaffolding for the residuals. **Resolved**:
  * **C-11** (plugin WASM HTTP host library SSRF) — new
    `pkg/netutil/ssrf.go` blocklist (loopback / RFC1918 / IPv6 ULA /
    link-local / cloud-metadata / CGNAT / reserved IPv4); rewritten
    `internal/plugin/hostlibrary/http.go` with custom
    `http.Transport.DialContext` that resolves the host, validates
    every candidate IP against the blocklist, and dials the chosen
    IP verbatim (DNS-rebinding defence); `CheckRedirect` re-validates
    every hop; scheme allow-list (`PLUGINS_HTTP_ALLOWED_SCHEMES`,
    default `https`); response-header **allow**list (Set-Cookie,
    Authorization, WWW-Authenticate, Proxy-Authenticate stripped
    on the way back to the plugin); `PLUGINS_HTTP_MAX_TIMEOUT`
    cap; `PLUGINS_HTTP_MAX_REDIRECTS` cap; operator allow-list
    `PLUGINS_HTTP_ALLOWED_HOSTS` that bypasses the private-IP block
    but never bypasses cloud-metadata IPs. Tests:
    `internal/plugin/hostlibrary/http_ssrf_security_test.go`
    (loopback, RFC1918, cloud-metadata, redirect-to-private,
    DNS-rebinding via fake resolver, timeout cap, redirect cap,
    response-header allowlist, operator-allowed host bypass,
    cloud-metadata cannot be bypassed) +
    `pkg/netutil/ssrf_test.go` (each blocklist category + public IP
    smoke tests).
  * **C-4** — `internal/api/middlewares/auth.go` `extractToken` now
    returns the source (header / query / cookie) and
    `sourceAllowsTokenType` enforces per-source policy: `?token=`
    accepts ONLY `glst_`; cookie accepts PASETO + glst_, NOT PAT;
    Authorization header accepts any. Failed transports are audited
    with `token_source_<kind>_not_allowed` and surfaced as "missing
    token" so an attacker cannot tell a wrong-source rejection from
    a missing credential. Tests:
    `internal/api/middlewares/auth_query_token_security_test.go`,
    updated `internal/api/router_security_auth_test.go`.
  * **C-10** — `pkg/tlsutil/ciphers.go::HardenServerConfig` applied
    in `internal/application/application.go` (HTTPS),
    `internal/application/container.go` (gRPC + multiplexer).
    Explicit TLS 1.2 cipher list (AEAD-only ECDHE-{ECDSA,RSA} +
    AES-GCM / ChaCha20-Poly1305); curves X25519 then P-256. Tests:
    `pkg/tlsutil/ciphers_test.go` (allowlist-only, denylist of
    CBC/RC4/static-RSA, X25519-first, defaults-on-zero-value,
    no-override-on-explicit, nil-input).
  * **T-8** — `pkg/auth/password.go` parameterised on
    `ActiveBcryptCost`; `SetDefaultBcryptCost` validates the
    operator value against `[MinBcryptCost=10, MaxBcryptCost=14]`
    and panics at boot on misconfiguration; default 13 matches the
    L2 minimum. `HashCost` exposes the stored cost so
    `internal/api/auth/login/handler.go` can rehash-on-login (and
    refuses to downgrade — a misconfigured operator who lowers the
    cost cannot weaken existing rows). `pkg/auth/dummy.go` runs a
    constant-time bcrypt verify on the non-existent-user path so
    login latency does not leak which logins exist. Tests:
    `pkg/auth/password_test.go` (range / cost-round-trip /
    HashCost-error path), `pkg/auth/dummy_test.go` (timing
    equality smoke test, no-panic-on-bad-input),
    `internal/api/auth/login/handler_test.go` (cost upgrade,
    no-downgrade).
  * **T-1 residual** — `internal/api/middlewares/security_headers.go`
    now applies `Cache-Control: no-store, no-cache, must-revalidate,
    private` + `Pragma: no-cache` to every response whose path
    starts with one of `SECURITY_SENSITIVE_PATH_PREFIXES` (default
    `/api/auth/`, `/api/profile/`, `/api/users/`, `/api/tokens/`)
    and does not already carry a `Cache-Control` set by the
    handler. Tests: 4 new cases in `security_headers_test.go`.
  * **T-10** — `.github/workflows/vuln-scan.yaml` (weekly + push to
    main + manual dispatch), pinned `govulncheck@v1.1.4`, source +
    binary modes, auto-files / auto-closes a GitHub issue per
    failing mode, input validation on the manual-dispatch version
    override to defend against workflow injection via that input.
  * **SECURITY.md** — top-level vulnerability-disclosure policy:
    `security@gameap.com`, 72h ack / 14d assessment / 90d
    disclosure window, severity-based fix SLAs, scope, demo
    deployment policy.
  * **C-8 (upload side)** — new `internal/api/filemanager/filemanagermime/Checker`
    (default allowlist mirrors the serve-side `inlineSafeMimes` +
    structured-text payloads; `FILES_UPLOAD_ALLOW_ARCHIVES` /
    `FILES_UPLOAD_ALLOW_BINARY` toggles; `FILES_UPLOAD_ALLOWED_MIMES`
    additive operator override). `internal/api/filemanager/upload/handler.go`
    sniffs the first 512 bytes via `bufio.Reader.Peek` +
    `http.DetectContentType` before the daemon transfer starts and
    emits `file.upload` audit with `detected_mime` + `reason=
    mime_not_allowed` on rejection. Tests: `filemanagermime/allowed_test.go`
    + `upload/handler_test.go::TestHandler_C8_*` (HTML-as-PNG
    rejected and audited, real PNG accepted, plain-text config
    accepted, ZIP refused by default, ZIP accepted when
    `AllowArchives=true`).
  * **PAT revoke** — `internal/api/tokens/deletetoken/handler.go`
    now writes a `pat:<id>` entry to the revocation denylist on
    every successful delete; `internal/api/middlewares/auth.go`
    `processPersonalAccessToken` checks the same identifier after
    the repository lookup so a stale repository cache cannot
    resurrect a revoked PAT. Identifier helper exported as
    `deletetoken.PATRevocationIdentifier`; middleware mirrors the
    shape via package-local `patRevocationIdentifier` to avoid the
    handler-import cycle.
  * **Re-auth helper** — `internal/api/base/reauth.go::VerifyCurrentPassword`
    (sentinels `ErrMissingCurrentPassword` / `ErrInvalidCurrentPassword` /
    `ErrReauthNotAvailable`; refuses PAT sessions; refuses sessions
    without a populated User.Password; emits `auth.reauth.success` /
    `auth.reauth.failure` audit). Tests: `internal/api/base/reauth_test.go`
    (correct password / missing / wrong / PAT session / unauth /
    session-without-hash). Handler integration deferred — the
    `current_password` field is declared on `posttoken.tokenInput`
    so the JSON contract is forward-compatible; bulk-rewriting the
    17-case `posttoken/handler_test.go` matrix is tracked as the
    Sprint 3 residual for S2.3.
  * **MFA-nudge scaffolding (S2.4)** — config
    (`AUTH_REQUIRE_MFA_FOR_ADMINS`, `AUTH_MFA_HARD_FAIL_DAYS`),
    new package `internal/services/mfanudge` (pure-logic
    `Recommendation` computation with fixed 24h `SnoozeDuration`
    and hard-fail boundary), audit events `auth.mfa.nudge.shown`,
    `auth.mfa.nudge.snoozed`, `auth.mfa.enrollment.required`,
    `auth.mfa.enrollment.completed`. Tests:
    `internal/services/mfanudge/service_test.go` (non-admin /
    has-2FA / operator-flag-off short-circuits, first-contact
    timestamps, snooze suppresses / expires, hard-fail boundary,
    snooze overridden by hard-fail, `MFAHardFailDays=0` disables
    escalation, days-remaining rounds up). **Residual**: DB
    migration (`users.mfa_nudge_first_shown_at` +
    `mfa_nudge_snoozed_until`), repository methods, login-flow
    integration, `POST /api/profile/mfa-nudge/snooze` endpoint and
    frontend modal — Sprint 3 follow-up. 4.3.1 stays 🟡 Partial
    until the integration lands.
  * **Idle-timeout scaffolding (S2.5)** — `pkg/auth/idle_tracker.go`
    (`IdleTracker` interface, `NoopIdleTracker`,
    `CacheIdleTracker` backed by `cache.Cache` with int64
    unix-nano values and TTL = idle ceiling). Config
    `AUTH_SESSION_IDLE_TIMEOUT` (default 30m) +
    `AUTH_SESSION_IDLE_UPDATE_FREQ` (5m, drives the probabilistic
    refresh that caps cache-write load). Tests:
    `pkg/auth/idle_tracker_test.go` (round-trip, missing entry,
    TTL=0 noop, expired entry). **Residual**: middleware wiring
    (session-PASETO-only filter, post-credentials check + refresh),
    WebSocket ping integration to keep long-lived connections
    alive, login-time first-record. 3.3.2 stays ❌ until middleware
    integration lands.

  Aggregate L2 conformance after this work is expected to climb from
  62% to **~78–82%** (final tally depends on whether the residuals
  above ship before the next re-audit). The §2.1 scoreboard is left
  unchanged in this entry pending a separate re-tally PR.

- **2026-05-28 — re-audit.** Verified the feature work landed since
  2026-05-18 (commits `2ed7be2` "remove legacy" through `b5bb958`
  "passwords security update"). **Resolved**: **C-1 / T-3** (the
  entire legacy daemon HTTP / binnapi code path was deleted —
  `internal/daemon/conn.go`, `internal/daemon/binnapi/*`,
  `internal/daemon/command_legacy.go`, `internal/api/daemonapi/*`,
  `internal/api/middlewares/daemon.go`, `internal/api/middlewares/daemon_grpc_guard.go`,
  `internal/application/legacy.go` and the corresponding test files
  — so the `InsecureSkipVerify: true` + `MinVersion: tls.VersionTLS10`
  client no longer exists in the codebase); **C-2 / T-1** (global
  `SecurityHeadersMiddleware` shipped — HSTS conditional on HTTPS,
  X-CTO, X-Frame-Options, Referrer-Policy, generated CSP with
  inline-script SHA-256 hashes harvested at boot from `index.html` +
  `streamsaver/mitm.html`, captcha-aware sources for reCAPTCHA /
  Turnstile, operator extras via `cfg.Security.CSP.Extra*`, master
  switch + report-only / verbatim-policy / report-URI knobs, fail-boot
  on missing static file, 17 unit tests; commits `37f5f34` "security
  headers" + `52edd5c` "update tests"). **New finding**: **C-11** —
  plugin WASM HTTP host library (`internal/plugin/hostlibrary/http.go:48`,
  wired at `internal/application/container.go:1804`) passes
  plugin-controlled URLs to `stdhttp.NewRequestWithContext` with no
  scheme allow-list / IP blocklist / redirect cap / response-header
  redaction / `TimeoutSeconds` clamp. Supply-chain SSRF pivot risk
  from a compromised plugin-store entry, mitigated only by AdminOnly
  install + SHA-256 hash verification. Demotes ASVS 12.6.1 and 5.2.8
  from ✅ to 🟡; adds top-10 row **T-11**. **Note**: `node.GdaemonAPIToken`
  column still exists in `internal/domain/node.go:25` and the
  repository layer still hashes it on write, but no non-test code
  reads it anymore — column is dormant pending a drop migration.
  Test catalogue updated: removed deleted `router_security_daemon_test.go`,
  `internal/api/middlewares/daemon_test.go`,
  `internal/api/middlewares/daemon_grpc_guard_test.go`; added
  `internal/api/middlewares/security_headers_test.go` (17 funcs),
  `auth_shorttoken_test.go`, `auth_twofactor_security_test.go`,
  `audit_capture_test.go`, `shorttoken_scope_test.go`,
  `router_security_password_policy_test.go`,
  `router_security_auditlog_test.go`. Re-verified **C-6** (now PAT +
  gRPC daemon key only; daemon HTTP token scope dropped), **C-10**
  (no `CipherSuites` configured anywhere), **C-4** (`?token=` still
  accepts any token type), **T-8** (`bcrypt.DefaultCost = 10`
  unchanged in `pkg/auth/password.go:12`). Recomputed §2.1 scoreboard
  from detail rows: overall **62% → 64%** (net of C-1 + C-2 closures
  minus C-11 downgrade; without C-11 the score would have been
  ~70%).
- **2026-05-18 — re-audit.** Verified the feature work landed since the
  last review (commits through `a90a785`). Resolved **C-5** (gRPC
  `gdaemon_api_key` now SHA-256 at rest + hash-then-`secureCompare`),
  **C-7** (setup key invalidated on first successful enroll), **C-9**
  (TOTP 2FA + bcrypt single-use recovery codes; residual: admin-MFA
  *enforcement* → 4.3.1 Partial). Closed **T-4**/**T-5** in §2.3.
  Reworded **C-4** (single-use `glst_` token shipped; query still
  accepts long-lived tokens), **C-8** (safe file serving mitigates the
  XSS vector; upload validation still open). Re-verified **C-1**,
  **C-2**, **C-6**, **C-10** unchanged. Recomputed the §2.1 scoreboard
  and every §5 chapter score directly from the detail rows (the prior
  summary had drifted); overall **58% → 62%**.
- **2026-05-16 — C-3 / T-2 resolved.** `internal/audit/` package +
  `RequestContextMiddleware`.
