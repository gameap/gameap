# OWASP ASVS 4.0.3 — Level 1 Conformance

This document is the gameap-api project's self-assessment against the
[OWASP Application Security Verification Standard 4.0.3](https://owasp.org/www-project-application-security-verification-standard/),
**Level 1 (Opportunistic)** baseline.

It is the project's initial security standard. Every requirement in scope is
either backed by an automated test (with file/function reference) or listed
as a known gap with a remediation plan.

---

## 1. Scope and intent

| Item | Value |
| --- | --- |
| Standard | OWASP ASVS 4.0.3 |
| Target verification level | L1 (Opportunistic) |
| Application type | Self-hosted REST/JSON API + gRPC daemon control plane |
| Primary trust boundaries | (a) public Internet → API HTTP server, (b) game daemon → API gRPC, (c) operator → admin endpoints, (d) plugin (WASM) → host runtime |
| Last reviewed | 2026-05-28 (Sprint 1 + 2 close-out: plugin WASM HTTP host library SSRF gap **C-11 resolved** via `pkg/netutil/ssrf.go` + custom-dial hardened host library; ?token= restricted to glst_ only (C-4); explicit TLS cipher list (C-10); bcrypt cost configurable defaulting to 13 (T-8) + non-existent-user timing-oracle dummy; `Cache-Control: no-store` on auth/profile/users/tokens paths; `govulncheck` weekly workflow; `SECURITY.md` published; file-upload MIME magic-byte allowlist (C-8); PAT revoke writes to denylist + middleware re-checks; re-auth helper `internal/api/base/reauth.go`; MFA-nudge scaffolding (config + service + audit events) and idle-timeout scaffolding (`pkg/auth/idle_tracker.go` + config). Full detail in `docs/security/ASVS_L2.md` §9. Prior: 2026-05-28 (HSTS/CSP/X-CTO/X-Frame-Options/Referrer-Policy global middleware; legacy daemon HTTP path removed). |
| Owners | gameap-api maintainers |

L1 is chosen as the starting baseline. Items above L1 are noted as
"Out of L1 scope" and may be picked up by future hardening work.

### How to read each entry

| Symbol | Meaning |
| --- | --- |
| ✅ Met | Control implemented and verified by an automated test |
| 🟡 Partial | Control partially implemented or relies on operator configuration |
| ❌ Not met | Gap; remediation tracked in section 6 or in a security audit document |
| ➖ N/A | Requirement does not apply to this application |

Each entry references concrete evidence using `path/to/file.go` and the
test function name where applicable.

### How to maintain this document

1. When a new security test is added, append the test name under the relevant
   ASVS requirement.
2. When a remediation lands, change Status to ✅ Met and link the merged PR.
3. Re-review on each minor release; bump the **Last reviewed** date above.
4. Per project policy (`CLAUDE.md` memory note): every security test file
   must have a header comment naming the OWASP API Top 10:2023 category, and
   every test function must repeat that category in its docstring.

---

## 2. Mapping: OWASP API Security Top 10:2023 ↔ tests ↔ ASVS

This mapping links the project's existing OWASP-labelled tests to ASVS
chapters. Use it as the entry point when a new finding needs to be slotted.

| API Top 10 (2023) | Primary ASVS chapters | Test files |
| --- | --- | --- |
| API1 — Broken Object Level Authorization | V4 (Access Control) | `internal/api/router_security_idor_test.go`, `router_security_idor_fuzz_test.go` |
| API2 — Broken Authentication | V2 (Authentication), V3 (Session) | `router_security_auth_test.go`, `router_security_auth_fuzz_test.go`; gRPC daemon auth covered by `internal/grpc/interceptors/auth_test.go` |
| API3 — Broken Object Property Level Authorization | V4 (Access Control) | `router_security_escalation_test.go`, `router_security_escalation_fuzz_test.go` |
| API4 — Unrestricted Resource Consumption | V11 (Business Logic), V12 (Files) | partially: `internal/api/filemanager/upload/handler_test.go` (size cap) |
| API5 — Broken Function Level Authorization | V4 (Access Control) | `router_security_escalation_test.go` |
| API6 — Unrestricted Access to Sensitive Business Flows | V11 (Business Logic) | _gap — see §6_ |
| API7 — Server-Side Request Forgery | V12 (Files), V13 (API) | ✅ Application-level outbound URLs config-derived. **Plugin WASM host library** (`internal/plugin/hostlibrary/http.go`) hardened 2026-05-28: `pkg/netutil/ssrf.go` blocklist + custom DialContext (DNS-rebinding safe) + scheme allowlist + response-header allowlist + `CheckRedirect` re-validation + `TimeoutSeconds` cap + cloud-metadata never bypassable. 12 SSRF-specific tests in `internal/plugin/hostlibrary/http_ssrf_security_test.go`. |
| API8 — Security Misconfiguration | V14 (Configuration) | `internal/enrollment/service_test.go`, `internal/api/nodes/enrollsetup/handler_test.go` (setup keys) |
| API9 — Improper Inventory Management | V14 (Configuration) | OpenAPI spec: `openapi/openapi.yaml` |
| API10 — Unsafe Consumption of APIs | V13 (API) | _gap — limited surface, no current consumers_ |

Shared helpers for all `router_security_*` tests live in
`internal/api/router_security_helpers_test.go`.

---

## 3. ASVS chapter conformance

Only requirements applicable to a JSON/HTTP + gRPC API are included. Pure
front-end requirements (HTML rendering, browser cookies, etc.) are marked
N/A unless the API issues them directly.

### V1 Architecture, Design and Threat Modeling

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 1.1.1 | SDLC includes security in every stage | 🟡 Partial | This document + the labelled `router_security_*` test suite are the codified evidence; threat modelling is informal. |
| 1.1.2 | Threat modelling for new features | ❌ Not met | No formal threat model document yet. Roadmap. |
| 1.1.3 | User stories include security acceptance criteria | 🟡 Partial | Security tests are written alongside features (see test file headers) but acceptance criteria are not formally tracked. |
| 1.4.1 | Trusted enforcement points (gateway, server-side) | ✅ Met | All authn/authz performed server-side via `internal/api/middlewares/auth.go`, `personal_access.go`, `shorttoken_scope.go`, `cors.go`, `https_redirect.go`, `security_headers.go`; gRPC daemon auth via `internal/grpc/interceptors/auth.go`. |
| 1.4.4 | Single vetted access-control mechanism | ✅ Met | RBAC concentrated in `internal/rbac/rbac.go`; ability checks routed via `base.RBAC` interface. |
| 1.5.1 | Trust boundaries documented | 🟡 Partial | Boundaries listed in §1 above; richer diagram in `docs/PROJECT_STRUCTURE.md` and `docs/gameap_architecture.svg`. |
| 1.8.1 | Sensitive data classified | ❌ Not met | Roadmap: classify daemon credentials, API tokens, user PII. |

### V2 Authentication

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 2.1.1 | Passwords are at least 12 characters | ✅ Met | `pkg/auth/policy.go` exports `MinPasswordLength = 12` and `auth.ValidatePassword` is enforced by every password-set entry point: `internal/api/users/postusers/input.go`, `internal/api/users/putuser/input.go`, `internal/api/profile/putprofile/input.go`. Login itself stays non-empty-only — ASVS does not mandate a length check on the login form. Verified by `pkg/auth/policy_test.go` and handler tests in the three packages above. |
| 2.1.2 | No truncation; allow ≥ 64 chars | ✅ Met | `pkg/auth/password.go` pre-hashes the password with SHA-256 + base64 before bcrypt, so the 72-byte bcrypt input limit no longer caps user input. `MaxPasswordLength = 128` (`pkg/auth/policy.go`) matches the ASVS upper bound. Round-tripped at 128 chars by `pkg/auth/password_test.go TestHashPassword_RoundTrip_128Chars`. Legacy raw-bcrypt hashes are still accepted by `VerifyPassword` and upgraded transparently on the next successful credential check (`internal/api/auth/login/handler.go`, `internal/api/profile/twofactor/{disable,recoverycodes}/handler.go`). Verified by `TestHandler_ServeHTTP_LegacyHashUpgradesOnLogin` (`internal/api/auth/login/handler_test.go`). |
| 2.1.3 | Allow Unicode and spaces (no truncation) | ✅ Met | `postusers.ToDomain` no longer trims the password before hashing — the exact validated bytes go through `auth.HashPassword`. The SHA-256 pre-hash keeps the digest input identical for any UTF-8 sequence (`pkg/auth/password_test.go TestHashPassword/unicode_password`). |
| 2.1.7 | Reject breached / common passwords | ✅ Met | Embedded common-password blocklist (SecLists top-1M filtered to ≥12 chars, lowercased, deduped, sorted, gzipped — ~46 K entries / ~324 KB on disk, ~3 MB RAM) consulted by `auth.ValidatePassword` (`pkg/auth/policy.go`, `pkg/auth/blocklist.go`). Asset committed in-repo; rebuild procedure documented in `pkg/auth/data/passwords/README.md`. Operator override `AUTH_ALLOW_WEAK_PASSWORDS=true` emits a startup `slog.Warn` and skips the check (length checks still apply). Login (`internal/api/auth/login/handler.go`) deliberately does NOT run the blocklist so pre-existing accounts with weak passwords keep working. Offline-only design — HIBP k-anonymity rejected to keep zero-egress deployments viable. Tests: `pkg/auth/blocklist_test.go`, `pkg/auth/policy_test.go`, `internal/api/router_security_password_policy_test.go`. |
| 2.1.9 | No composition rules ("must contain X") | ✅ Met | None imposed. |
| 2.2.1 | Anti-automation on credential test endpoints | ✅ Met | Two layers: `LoginRateLimitMiddleware` (`internal/api/middlewares/login_ratelimit.go`) caps failed `/api/auth/login` attempts at 20/IP and 5/username per 15 min (429); and a captcha (`internal/services/captcha/service.go` — reCAPTCHA v2/v3 / Turnstile) verified *before* the user store is touched (`internal/api/auth/login/handler.go:87-95`). Verified by `TestRouterSecurity_API2_LoginBruteForceProtection`, `TestLoginRateLimitMiddleware_*` (9 unit tests), `internal/services/captcha/service_test.go`. |
| 2.2.2 | Lockout / similar after failures | ✅ Met | Same rate-limit middleware (sliding-window TTL self-recovers); the 2FA-verify endpoint additionally enforces a per-challenge 5-attempt budget (`internal/api/auth/twofactorverify/handler.go:26`). |
| 2.2.3 | Notify users of significant security events | ❌ Not met | Roadmap. |
| 2.3.1 | System-generated initial passwords are random and changeable | ➖ N/A | No system-issued user passwords; admins set them. |
| 2.4.1 | Passwords stored using approved KDF | ✅ Met | bcrypt over a SHA-256 pre-hash (`pkg/auth/password.go`). Cost parameterised via `AUTH_BCRYPT_COST` (default 13, range 10–14) — `SetDefaultBcryptCost` panics at boot on misconfiguration so a typo cannot weaken the default below the project floor (2026-05-28, T-8 closed). Login transparently rehashes any hash whose stored cost is below the configured target via `auth.HashCost` (refuses to downgrade — a misconfigured operator who lowers the cost cannot weaken existing rows). Tests: `pkg/auth/password_test.go::TestSetDefaultBcryptCost_*` / `TestHashPassword_RespectsActiveCost` / `TestHashCost_ReturnsErrorForGarbage`; `internal/api/auth/login/handler_test.go::TestHandler_ServeHTTP_RehashesOnBcryptCostUpgrade` / `_DoesNotDowngradeBcryptCost`. |
| 2.4.2 | Salt is random and unique | ✅ Met | bcrypt salt is per-hash. |
| 2.5.1 | Password recovery does not reveal stored hash | 🟡 Partial | No recovery flow yet. Once added, must comply. |
| 2.5.4 | No default credentials shipped | 🟡 Partial | Setup keys are random or operator-supplied; tests live in `internal/enrollment/service_test.go` and `internal/api/nodes/enrollsetup/handler_test.go` (`TestEnrollSetup_RejectsInvalidSetupKey` and friends). |
| 2.5.5 | Forgot-password tokens are single-use, time-bound | ➖ N/A | No reset flow yet. |
| 2.6.x | Out-of-band authenticators (SMS, email link) | ➖ N/A | Not implemented. |
| 2.7.x | OTP / lookup secrets / TOTP | ✅ Met | RFC-6238 TOTP second factor with 10 single-use bcrypt-hashed recovery codes (lookup secrets). Secret AES-256-GCM-encrypted at rest, replay-locked last-used step, scope-confined `g2fa_` challenge token, per-challenge 5-attempt budget. `pkg/twofactor/`, `internal/api/auth/twofactorverify/handler.go`, `internal/api/auth/login/handler.go:139-216`, enrollment under `internal/api/profile/twofactor/`. Tests: `pkg/twofactor/twofactor_test.go`, `internal/api/middlewares/auth_twofactor_security_test.go`. (Opt-in per user; admin-MFA *enforcement* still on the roadmap — see 4.3.1.) |
| 2.8.1 | Single-factor cryptographic auth tied to a device | ✅ Met | Daemon mTLS path: `internal/grpc/interceptors/auth.go:101` (`verifyMTLS`). |
| 2.9.1 | Cryptographic key material protected | ✅ Met | PASETO key validated for length: `pkg/auth/paseto.go:18` (32-byte symmetric key). |
| 2.9.2 | Verifiers are stored as one-way values | ✅ Met | All bearer credentials are stored hashed: passwords via bcrypt over a SHA-256 pre-hash (`pkg/auth/password.go`); PATs via SHA-256 with constant-time comparison (`internal/api/middlewares/auth.go:313` using `subtle.ConstantTimeCompare`); gRPC `gdaemon_api_key` via SHA-256 with constant-time compare (`internal/grpc/interceptors/auth.go:185`, write at `internal/enrollment/service.go:107`). Migration `008_hash_gdaemon_api_keys.go` covers existing rows in mysql/postgres/sqlite. Verified by `TestRouterSecurity_API2_PATSecretMustBeOpaque`. |
| 2.10.1 | Secrets ≥ 128 bits | ✅ Met | PAT secret 48 random bytes (`internal/api/tokens/posttoken/handler.go`); gRPC daemon API key minted at enrollment (`internal/enrollment/service.go`); PASETO local key 32 bytes. |

Authentication tests:
`TestRouterSecurity_API2_BrokenAuthentication`,
`TestRouterSecurity_API2_TokenSchemes`,
`TestRouterSecurity_API2_TokenViaQueryAndCookie`,
`TestRouterSecurity_API2_UserDeletedAfterTokenIssue`,
`TestRouterSecurity_API2_PATSecretMustBeOpaque`,
`TestRouterSecurity_API2_LoginBruteForceProtection`,
`TestRouterSecurity_API2_LogoutInvalidatesToken`,
`TestRouterSecurity_API2_LogoutRequiresAuth`,
fuzz: `FuzzAuthMiddleware_AuthorizationHeader`, `FuzzAuthMiddleware_TokenParsing`,
`FuzzAuthMiddleware_AdminEndpointBypass` — all in `internal/api/router_security_auth*_test.go`. gRPC daemon authentication regressions are exercised by `internal/grpc/interceptors/auth_test.go` and `internal/enrollment/service_test.go`.

### V3 Session Management

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 3.2.1 | Sessions tokens have ≥ 64 bits entropy | ✅ Met | PAT 48 random bytes; PASETO v4.local symmetric encryption; daemon token 64 chars. |
| 3.2.2 | Tokens generated using approved CSPRNG | ✅ Met | `pkg/strings/CryptoRandomString` uses `crypto/rand`. |
| 3.2.3 | Token issued only after successful authentication | ✅ Met | `internal/api/auth/login/handler.go` issues only after `auth.VerifyPassword`. |
| 3.3.1 | Logout invalidates session | ✅ Met | `POST /api/auth/logout` (`internal/api/auth/logout/handler.go`) marks the bearer token as revoked; the auth middleware checks the denylist (`auth.TokenRevocation` / `CacheRevocation` in `pkg/auth/revocation*.go`) on every request. Verified by `TestRouterSecurity_API2_LogoutInvalidatesToken` and `TestRouterSecurity_API2_LogoutRequiresAuth`. |
| 3.3.2 | Idle session timeout | ❌ Not met | Roadmap. |
| 3.3.3 | Absolute timeout | ✅ Met | PASETO `exp` set in `pkg/auth/paseto.go:54`; default 24 h, max 7 d for "remember me" (`internal/api/auth/login/handler.go:19`, reduced from 30 d). Expiry verified by `TestRouterSecurity_API2_BrokenAuthentication`. |
| 3.3.4 | Session re-binding on privilege change | 🟡 Partial | RBAC role changes propagate after cache TTL expires; `TestRouterSecurity_API5_Escalation_RemovedAdminRoleLosesAccess` covers this. |
| 3.4.1 | Cookies marked Secure | 🟡 Partial | Tokens primarily transported via Authorization header. Cookie path read in `internal/api/middlewares/auth.go:146`; the API does not currently set cookies itself. Frontend operators set cookie attributes. |
| 3.4.2 | Cookies marked HttpOnly | 🟡 Partial | Same as 3.4.1. |
| 3.4.3 | SameSite | 🟡 Partial | Same as 3.4.1. |
| 3.5.1 | Logout endpoint accessible from all pages | ✅ Met | `POST /api/auth/logout` registered in `internal/api/router.go`; documented in `openapi/paths/auth.yaml`. |
| 3.5.2 | Token-based sessions: no use after expiry | ✅ Met | `TestRouterSecurity_API2_BrokenAuthentication` covers expired PASETO. |
| 3.5.3 | Stateless tokens digitally signed/encrypted | ✅ Met | PASETO v4.local (encrypted + authenticated) by default; JWT HS384 fallback in `pkg/auth/jwt.go`. |
| 3.7.1 | Re-auth before sensitive operations | ❌ Not met | Roadmap. |

### V4 Access Control

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 4.1.1 | Access controls enforced server-side | ✅ Met | All authz in middleware + handler RBAC; covered by `TestRouterSecurity_API5_BFLA_RegularUserRejected`, `_UnauthenticatedRejected`, `_AdminAllowed` (26 admin endpoints). |
| 4.1.2 | Attributes used for AC are not user-controllable except for authn data | ✅ Met | `TestRouterSecurity_API3_Escalation_RegularUserCannotEditOtherUsers`, `FuzzPutUserBody_MassAssignment`. |
| 4.1.3 | Principle of least privilege | ✅ Met | Granular abilities: `internal/domain/rbac.go`; assignment validated: `internal/domain/auth.go:157-166`. |
| 4.1.4 | Principle of deny by default | ✅ Met | Auth middleware rejects on missing token (`TestRouterSecurity_API2_BrokenAuthentication`); admin middleware rejects on missing ability. |
| 4.1.5 | AC logs failures, alerts on repeated denials | 🟡 Partial | AC denials now emit structured `access.denied` audit events at the single RBAC choke point + admin gate (`internal/audit/`, `internal/api/servers/base/abilitychecker.go`); automated alerting on repeats remains an operator/SIEM concern. |
| 4.2.1 | Sensitive data and APIs are protected from IDOR | ✅ Met | `TestRouterSecurity_API1_BOLA_*` (7 cases) + `FuzzServerIDPathParam_DoesNotLeakOtherServer`, `FuzzFileManagerPath_*`. |
| 4.2.2 | CSRF defenses for state-changing operations | 🟡 Partial | API uses Authorization header (not cookie auth) by default — mitigates CSRF. Cookie path exists for browsers; `SameSite` not enforced server-side. Roadmap: documented as gap. |
| 4.3.1 | Admin interfaces use MFA | 🟡 Partial | TOTP 2FA available and enforced at login once an account enables it (see 2.7.x), but opt-in — no `require_mfa_for_admins` policy yet. Roadmap. |
| 4.3.2 | Admin functions only accessible to admins | ✅ Met | `TestRouterSecurity_API5_BFLA_*` and `IsAdminMiddleware` (`internal/api/middlewares/auth.go:327`). |
| 4.3.3 | Sensitive admin operations require step-up auth | ❌ Not met | Roadmap. |

### V5 Validation, Sanitization and Encoding

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 5.1.1 | Input validation enforced on a trusted layer | ✅ Met | Centralised via `pkg/api/reader.go` (`api.NewInputReader`) + per-handler `Validate()`. |
| 5.1.2 | HTTP parameter pollution defence | ✅ Met | Standard `net/http` rejects duplicate path params; query param parsing reads first value. |
| 5.1.3 | All inputs validated (type, length, range, allow-list) | ✅ Met | Sort fields are now allow-listed at the filter-library boundary via `filters.ParseUserSort` (`internal/filters/order.go`); the two user-controlled handlers (`internal/api/servers/getservers/input.go`, `internal/api/daemontasks/getdaemontasks/handler.go`) reject any field not in their explicit map. Other inputs validated per-handler. |
| 5.1.4 | Structured data validated against schema | ✅ Met | OpenAPI spec `openapi/openapi.yaml`. |
| 5.1.5 | URL redirects validated against allow-list | ➖ N/A | No redirect endpoints accept user-supplied targets. |
| 5.2.1 | Untrusted HTML sanitized | ➖ N/A | JSON API only; no server-side HTML rendering. |
| 5.2.5 | Markdown / template safety | ➖ N/A | Templates not rendered. |
| 5.3.1 | Output encoding contextual | ✅ Met | `encoding/json` default escaping. |
| 5.3.4 | Parameterized queries (no string concatenation) | ✅ Met | All repositories use Squirrel + placeholder binding (`internal/repositories/mysql/*.go`, `postgres/*.go`). |
| 5.3.5 | ORM/SQLi via dynamic identifiers prevented | ✅ Met | `filters.ParseUserSort` (`internal/filters/order.go`) returns `ErrInvalidSortField` when a sort key is not in the caller-supplied allow-list. SQL-injection payloads in the `sort` parameter are unit-tested in `internal/filters/order_test.go::TestParseUserSort`. |
| 5.3.6 | LDAP queries protected | ➖ N/A | No LDAP. |
| 5.3.7 | OS command construction protected | ✅ Met | No `exec.Command` with user input identified. |
| 5.3.8 | XML/XPath/XXE | ➖ N/A | No XML parsing. |
| 5.5.1 | Serialization untrusted data prevented | ✅ Met | JSON only; YAML only for trusted export (`goccy/go-yaml`). No `gob`, no `encoding/gob`. |
| 5.5.2 | Insecure deserialization libs avoided | ✅ Met | Standard `encoding/json`. |

### V7 Error Handling and Logging

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 7.1.1 | No sensitive data in logs (passwords, tokens) | 🟡 Partial | Login handler does not log `Password`; audit `reason` is a stable enum and never carries secrets. Residual: tokens passed via `?token=` (`internal/api/middlewares/auth.go:227-230`) may still be captured by upstream proxy logs (a single-use `glst_` token is now the safer URL credential). |
| 7.1.2 | No credentials or secrets logged | 🟡 Partial | Same as 7.1.1; the audit package never logs token values or file contents. |
| 7.2.1 | Sufficient logging for security events | ✅ Met | Structured `slog` audit stream (`component=audit`) over auth success/failure/blocked, token rejection, RBAC denials and 13 sensitive-op handlers (`internal/audit/`, wired via `RequestContextMiddleware`). Config `AUDIT_ENABLED`. Tests: `internal/audit/*_test.go`, `internal/api/router_security_auditlog_test.go`. |
| 7.2.2 | Log messages include enough context (user, IP, timestamp) | ✅ Met | Every audit record carries actor (id/login/auth-method), client IP, user-agent, HTTP method/path, timestamp and a sanitized `X-Request-Id` (`internal/audit/{event,middleware,context}.go`). |
| 7.4.1 | Generic error messages to clients on 5xx | ✅ Met | `pkg/api/responder.go:114` returns `http.StatusText(code)` for ≥ 500. |
| 7.4.2 | Sensitive details only in server logs | ✅ Met | Detailed errors wrapped via `errors.WithMessage` server-side; `pkg/api/responder.go` strips on 5xx. |
| 7.4.3 | Recovery middleware to prevent panics from leaking | ✅ Met | `internal/api/middlewares/recovery.go` + `recovery_test.go`. |

### V8 Data Protection

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 8.1.1 | No sensitive data in URLs | ✅ Met | `internal/api/middlewares/auth.go::sourceAllowsTokenType` enforces per-source allow-list (2026-05-28, C-4 closed): `?token=` accepts ONLY the single-use ≤10 s `glst_` short-lived token; long-lived PASETO/PAT/JWT in the query string are rejected as "missing token" + audited with `token_source_query_not_allowed`. Cookie path also restricted to PASETO + glst_ (PAT API credentials must use Authorization header). Tests: `internal/api/middlewares/auth_query_token_security_test.go`, `router_security_auth_test.go::TestRouterSecurity_API2_TokenViaQueryAndCookie`. |
| 8.2.1 | Browser caching of sensitive responses controlled | ✅ Met | `SecurityHeadersMiddleware` (2026-05-28, T-1 residual closed) emits `Cache-Control: no-store, no-cache, must-revalidate, private` + `Pragma: no-cache` for any request whose path begins with one of `SECURITY_SENSITIVE_PATH_PREFIXES` (default `/api/auth/`, `/api/profile/`, `/api/users/`, `/api/tokens/`). Downstream handlers that emit their own `Cache-Control` win (idempotent guard). Tests: `security_headers_test.go::TestSecurityHeaders_CacheControl_*` (4 cases: sensitive paths, public paths, handler override, empty-prefix list inert). |
| 8.3.1 | Sensitive data masked in responses | ✅ Met | Daemon API key, password and login fields removed from `nodeResponse` in `internal/api/nodes/{getnode,putnode}/response.go`; `getdaemonstatus/response.go` returns a `has_api_key` boolean instead of the key value. Daemon API tokens are stored as SHA-256 hashes (see 2.9.2). Verified by `TestRouterSecurity_API3_NodeResponseDoesNotLeakDaemonSecrets`. |
| 8.3.4 | Sensitive data classified and protected | ❌ Not met | Roadmap (V1.8.1 too). |

### V9 Communications

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 9.1.1 | TLS for all inbound and outbound traffic | 🟡 Partial | HTTPS redirect available: `internal/api/middlewares/https_redirect.go` + `https_redirect_test.go`. Enforcement is operator config. |
| 9.1.2 | Strong TLS configuration | ✅ Met | `pkg/tlsutil/ciphers.go::HardenServerConfig` (2026-05-28, C-10 closed) applied to every TLS listener (HTTPS in `application.go`, gRPC + multiplexer in `container.go`). `MinVersion: tls.VersionTLS12`; explicit AEAD-only cipher list (ECDHE-{ECDSA,RSA} + AES-GCM / ChaCha20-Poly1305, no CBC/RC4/3DES/static-RSA); X25519-first curves. Tests: `pkg/tlsutil/ciphers_test.go`. |
| 9.1.3 | TLS used for authenticated connections | ✅ Met | gRPC supports mTLS: `internal/grpc/interceptors/auth.go:101` (`verifyMTLS`); `RequireMTLS` operator-controlled. |
| 9.2.1 | Server certificates validated | ✅ Met for outbound API consumers (standard Go `http.Client`). |
| 9.2.3 | Encrypted connections to external services | ✅ Met | Outbound calls use HTTPS where remote supports it; e.g. `internal/services/globalapi.go`. |

### V10 Malicious Code

ASVS V10 is largely about software supply chain assurance. For L1, we rely on
ecosystem tooling (Go module checksum DB, `go.sum`). No internal evidence
required at this level.

### V11 Business Logic

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 11.1.1 | Business logic enforces a sequence of valid steps | 🟡 Partial | Setup-key / enrollment flow validated: `TestRouterSecurity_API8_EnrollmentSetupKeyValidation`. |
| 11.1.2 | Business logic limits use to expected actors | ✅ Met | RBAC + per-server access control: `internal/rbac/rbac.go`, `internal/api/servers/base/serverfinder.go`. |
| 11.1.4 | Anti-automation on critical flows | ✅ Met | Login is rate-limited (see 2.2.1). Other write-heavy flows are still uncapped — tracked separately on the roadmap. |

### V12 Files and Resources

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 12.1.1 | Maximum file size enforced | ✅ Met | 100 MB hard cap via `http.MaxBytesReader` in `internal/api/filemanager/upload/handler.go:117-126`. |
| 12.1.2 | Files compressed and structured before processing | ➖ N/A | No archive auto-extraction in API. |
| 12.1.3 | Storage quotas enforced per user | ❌ Not met | Roadmap. |
| 12.2.1 | File type allow-listing | 🟡 Partial | Filename validated; no MIME / magic-byte verification. Roadmap. |
| 12.3.1 | User-supplied path canonicalised | ✅ Met | Hardened `ValidatePath` / `ValidateFilename` in `internal/api/filemanager/filemanagerpath/path.go` (rejects NUL, backslash and any `..` component before the value is joined under `node.WorkPath/server.Dir`); fuzz coverage `FuzzFileManagerPath_DoesNotBypassAuthorization`, `FuzzFileManagerPath_OwnServer_DoesNotLeakOtherServerFiles`; unit `filemanagerpath/path_test.go`. |
| 12.3.2 | Files written outside intended directory rejected | ✅ Met | Same; per project convention `os.Root` is used for directory-limited filesystem access (`CLAUDE.md`). |
| 12.3.3 | File metadata not used for authz decisions | ✅ Met | Authorization is RBAC-based, not metadata-based. |
| 12.4.1 | Uploaded file content type and signature validated | ✅ Met | `internal/api/filemanager/filemanagermime/Checker` (2026-05-28, C-8 closed) + 512-byte sniff via `bufio.Reader.Peek` + `http.DetectContentType` in `upload/handler.go`. Default allowlist mirrors serve-side `inlineSafeMimes` + structured-text formats; operator-tunable via `FILES_UPLOAD_ALLOWED_MIMES` / `FILES_UPLOAD_ALLOW_ARCHIVES` / `FILES_UPLOAD_ALLOW_BINARY`. Rejected uploads emit `file.upload` audit with `detected_mime` + `reason=mime_not_allowed`. Tests: `filemanagermime/allowed_test.go`, `upload/handler_test.go::TestHandler_C8_*`. |
| 12.5.1 | Uploaded files served from a different domain or with safe headers | ✅ Met | `SafeContentHeaders` (`internal/api/filemanager/filemanagerhttp/headers.go`) sets `X-Content-Type-Options: nosniff` + `Content-Security-Policy: sandbox` on every served file, serves only an inert-MIME allowlist `inline` (SVG excluded), forces everything else to an opaque `attachment` (RFC 2231/6266). Test: `filemanagerhttp/headers_test.go`. |
| 12.6.1 | Untrusted SSRF blocked | 🟡 Partial | Application-level outbound URLs come from configuration, not user input (`internal/services/globalapi.go`, `pluginstore/service.go`). **Gap**: plugin WASM HTTP host library (`internal/plugin/hostlibrary/http.go:48`) accepts plugin-controlled URLs verbatim with no scheme allow-list / IP blocklist / redirect cap / response-header redaction; mitigated by AdminOnly install + SHA-256 hash verification of the WASM blob — see §6 roadmap. |

### V13 API and Web Service

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 13.1.1 | API uses defined schema (OpenAPI/GraphQL) | ✅ Met | `openapi/openapi.yaml`. |
| 13.1.3 | API consumes only declared content types | ✅ Met | JSON enforced in handlers via decoder; alternate types rejected with 4xx. |
| 13.1.4 | Different processing paths for different content types | ✅ Met | `/api/*` (JSON) and the gRPC daemon listener use distinct middleware chains (`internal/api/router.go`, `internal/grpc/server.go`, `internal/grpc/interceptors/auth.go`). |
| 13.1.5 | Requests from non-browser clients have no implicit trust | ✅ Met | All `/api/*` enforces auth. |
| 13.2.1 | REST endpoints use the correct HTTP verbs | ✅ Met | Verb-specific handlers; OpenAPI spec enforces. |
| 13.2.2 | JSON schema validation | 🟡 Partial | Per-handler input validation; no automatic OpenAPI request validation in router. Roadmap. |
| 13.2.3 | RESTful authentication tokens carry minimum data | ✅ Met | PASETO/JWT subject is `user:login:<login>`; PAT carries only ID + opaque secret. |
| 13.2.4 | REST services protected against CSRF (where cookies used) | 🟡 Partial | Auth via Authorization header by default mitigates; cookie auth path lacks SameSite. |
| 13.4.1 | GraphQL specifics | ➖ N/A | No GraphQL. |

### V14 Configuration

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 14.1.1 | Build / deployment process is documented and repeatable | ✅ Met | `Makefile`, `Dockerfile`, `DOCKER.md`. |
| 14.1.2 | Compiler flags / hardening enabled | 🟡 Partial | Standard Go toolchain; no explicit hardening flags documented. |
| 14.1.3 | Dependencies up to date and patched | ✅ Met | `go.mod`/`go.sum` version-pinned; `.github/workflows/vuln-scan.yaml` (2026-05-28, T-10 closed) runs `govulncheck@v1.7.0` against source + binary modes weekly + on push to main + manual dispatch, auto-files / auto-closes a GitHub issue per failing mode, and validates the manual-dispatch version override against a semver pattern to defend against workflow-injection via that input. |
| 14.1.5 | Software Bill of Materials | 🟡 Partial | `go.mod` + `govulncheck` cover dependency awareness; CycloneDX-format SBOM generation in the release pipeline remains a Sprint 3 roadmap item. |
| 14.2.1 | Components are inventoried | 🟡 Partial | Implicit via `go.mod`. CycloneDX SBOM still deferred (Sprint 3). |
| 14.2.6 | Vulnerability-disclosure programme | ✅ Met | Top-level `SECURITY.md` (2026-05-28) defines reporting channel (`security@gameap.com` / GitHub Security Advisory), 72h ack / 14d assessment / 90d coordinated-disclosure SLA, severity-based fix timelines, in/out-of-scope list, demo-deployment testing policy. |
| 14.2.5 | Default credentials removed | ✅ Met | Setup keys are operator-supplied or random; tested by `internal/enrollment/service_test.go` and `internal/api/nodes/enrollsetup/handler_test.go`. |
| 14.3.1 | Debug features disabled in production | 🟡 Partial | Operator config. Roadmap: explicit production flag check. |
| 14.3.2 | Security HTTP headers (HSTS, X-Frame-Options, X-Content-Type-Options, CSP) | ✅ Met | Global `SecurityHeadersMiddleware` (`internal/api/middlewares/security_headers.go`) emits HSTS (HTTPS-only via `r.TLS != nil` / `X-Forwarded-Proto: https` / `TLS.ForceHTTPS`), `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy` and a generated CSP (inline-script SHA-256 hashes extracted at boot from `index.html` + `streamsaver/mitm.html`; captcha-aware sources for reCAPTCHA / Turnstile; operator extras via `cfg.Security.CSP.Extra*`). Env-tunable via `cfg.Security.*` (`internal/config/config.go:180-221`). Tests: 17 funcs in `internal/api/middlewares/security_headers_test.go`. |
| 14.3.3 | Cross-Origin Resource Policy / referrer policy | 🟡 Partial | `Referrer-Policy` (default `strict-origin-when-cross-origin`) emitted by `SecurityHeadersMiddleware`. COOP / COEP not yet set. |
| 14.4.1 | Every response has Content-Type | ✅ Met | Set by `pkg/api/responder.go`. |
| 14.4.2 | Each response specifies safe character set | ✅ Met | JSON encoder default UTF-8. |
| 14.4.3 | Content-Type allowlist applied | ✅ Met | Handlers reject unexpected types. |
| 14.4.4 | CORS allow-list scoped to trusted domains | ✅ Met | `internal/api/middlewares/cors.go::deriveDefaultOrigin` picks the scheme from `TLS.ForceHTTPS` (no longer hardcoded to `http://`); operators can override the auto-derived origin via the `HTTP_ALLOWED_ORIGINS` env var. Verified by `TestNewCORSMiddleware_HTTPSWhenForceHTTPS`, `_RejectsHTTPOriginWhenForceHTTPS`, `_ExplicitAllowedOriginsWinsOverAutoDerived` in `internal/api/middlewares/cors_test.go`. |
| 14.4.5 | HTTP methods restricted | ✅ Met | Verb routing; CORS preflight handled by `rs/cors`. |
| 14.4.6 | Anti-clickjacking | ✅ Met | `X-Frame-Options: SAMEORIGIN` (default) + CSP `frame-ancestors 'self'` via `SecurityHeadersMiddleware`. |
| 14.4.7 | `X-Content-Type-Options: nosniff` | ✅ Met | `X-Content-Type-Options: nosniff` via `SecurityHeadersMiddleware` globally; file-download path additionally sets it locally (see 12.5.1). |
| 14.5.1 | Server rejects HTTP methods not used | ✅ Met | Router matches method explicitly. |
| 14.5.2 | Domain-name validation when constructing URLs | ✅ Met | All outbound URLs come from config. |
| 14.5.3 | CORS Origin verified server-side | ✅ Met | `rs/cors` validates against the configured allow-list. |

---

## 4. Test catalogue (evidence inventory)

| Test file | Category covered | Standard tests | Fuzz targets |
| --- | --- | --- | --- |
| `internal/api/router_security_idor_test.go` | API1:2023 BOLA / IDOR | 7 | — |
| `internal/api/router_security_idor_fuzz_test.go` | API1:2023 BOLA / IDOR | — | 3 |
| `internal/api/router_security_auth_test.go` | API2:2023 Broken Authentication | 8 (+ logout & brute-force regressions) | — |
| `internal/api/router_security_auth_fuzz_test.go` | API2:2023 Broken Authentication | — | 3 |
| `internal/api/router_security_escalation_test.go` | API3:2023 + API5:2023 | 10 (+ daemon-secret-leak regression) | — |
| `internal/api/router_security_escalation_fuzz_test.go` | API3:2023 + API5:2023 | — | 3 |
| `internal/api/router_security_test.go` | API1/API5 token-ability + admin gating | 2 | — |
| `internal/api/router_security_helpers_test.go` | shared fixtures | (helpers) | — |

Middleware-level unit tests:

- `internal/api/middlewares/auth_test.go`
- `internal/api/middlewares/auth_shorttoken_test.go`
- `internal/api/middlewares/auth_twofactor_security_test.go`
- `internal/api/middlewares/audit_capture_test.go`
- `internal/api/middlewares/personal_access_test.go`
- `internal/api/middlewares/cors_test.go` (incl. HTTPS scheme + explicit allow-list cases)
- `internal/api/middlewares/https_redirect_test.go`
- `internal/api/middlewares/recovery_test.go`
- `internal/api/middlewares/login_ratelimit_test.go` (9 cases: per-IP / per-username / reset-on-success / X-Real-IP / etc.)
- `internal/api/middlewares/security_headers_test.go` (17 funcs: defaults, master-switch, HSTS emission/format/max-age=0, CSP report-only, verbatim-policy override, captcha provider matrix, extra-source merging, core directives, downstream override, embedded-FS happy path + missing-file failure, inline-script discovery, report-URI, tokenizer error)
- `internal/api/middlewares/shorttoken_scope_test.go`
- `internal/grpc/interceptors/auth_test.go` (mTLS + gRPC API-key constant-time compare)

Library-level unit tests:

- `internal/filters/order_test.go::TestParseUserSort` (16 cases incl. `id;DROP TABLE users--`)

2FA / captcha / audit / safe file serving (added since 2026-04-28):

- `pkg/twofactor/twofactor_test.go` — TOTP skew/replay, AES-256-GCM round-trip, single-use bcrypt recovery codes
- `internal/api/auth/twofactorverify/handler_test.go`, `internal/api/middlewares/auth_twofactor_security_test.go` — challenge-token scope confinement + attempt budget
- `internal/api/profile/twofactor/{setup,confirm,disable,recoverycodes}/handler_test.go`
- `internal/api/auth/shorttoken/handler_test.go` — ≤10 s single-use URL token
- `internal/services/captcha/service_test.go` — provider verify, v3 score, fail-open/closed
- `internal/audit/{audit,logger,middleware}_test.go`, `internal/api/router_security_auditlog_test.go` — structured security audit log
- `internal/api/filemanager/filemanagerpath/path_test.go`, `internal/api/filemanager/filemanagerhttp/headers_test.go` — path canonicalisation + safe serving headers

Run all standard security tests:

```bash
go test ./internal/api/... -run '^TestRouterSecurity_'
```

Run a fuzz target (example):

```bash
go test -run NONE -fuzz=FuzzAuthMiddleware_AuthorizationHeader -fuzztime=30s ./internal/api/
```

Seed corpora are exercised automatically when tests are run without `-fuzz`.

---

## 5. Definition of "evidence"

For an ASVS requirement to be marked ✅ Met it must satisfy at least one of:

1. An automated test asserts the control behaves correctly under both the
   happy path and at least one negative case.
2. The control is implemented at a single, vetted enforcement point that is
   covered by integration tests for any handler that uses it (e.g. the auth
   middleware).
3. The control is a property of a third-party library that is itself well
   established (e.g. `crypto/rand`, `bcrypt`).

For 🟡 Partial: the control is implemented but lacks one of the above
guarantees, or its enforcement depends on operator configuration.

For ❌ Not met: no implementation exists.

For ➖ N/A: the requirement does not apply to a JSON/HTTP + gRPC API used
by this project (e.g. browser cookie attributes when cookies are not
issued by the API).

---

## 6. Open items / roadmap

### Resolved since 2026-04-28

| Description | ASVS req | Resolved |
| --- | --- | --- |
| TOTP 2FA + single-use bcrypt recovery codes | 2.7.x | 2026-05-18 |
| Structured security audit logging (auth, AC denials, sensitive ops) | 7.2.1, 7.2.2, 4.1.5 (partial) | 2026-05-16 |
| Safe file-serving headers (`nosniff`, CSP `sandbox`, inert-MIME allowlist) | 12.5.1 | 2026-05-18 |
| Captcha-gated login (reCAPTCHA / Turnstile) | 2.2.1 | 2026-05-18 |
| Password policy (min 12, max 128, no truncation, transparent rehash of legacy bcrypt hashes) | 2.1.1, 2.1.2, 2.1.3 | 2026-05-20 |
| Common-password blocklist (SecLists top-1M; offline; operator override via `AUTH_ALLOW_WEAK_PASSWORDS`) | 2.1.7 | 2026-05-27 |
| Global security HTTP headers (HSTS, CSP with inline-script SHA-256 hashes, X-CTO, X-Frame-Options, Referrer-Policy) | 14.3.2, 14.4.6, 14.4.7 | 2026-05-28 |
| Legacy daemon HTTP / binnapi path removed entirely (TLS 1.0 + `InsecureSkipVerify` client deleted along with the code) | 9.1.2, 9.2.1 | 2026-05-28 |
| Plugin WASM HTTP host library SSRF defences — `pkg/netutil/ssrf.go` blocklist + DNS-rebinding-safe custom DialContext + scheme allowlist + response-header allowlist + redirect re-validation + timeout cap (C-11) | 12.6.1, 5.2.8, 1.4.5 | 2026-05-28 |
| Per-source token extraction policy — `?token=` accepts only `glst_`; cookie rejects PAT (C-4) | 8.1.1, 3.1.1 | 2026-05-28 |
| Explicit TLS cipher-suite + curve policy via `pkg/tlsutil.HardenServerConfig` (C-10) | 9.1.2 | 2026-05-28 |
| bcrypt cost 13 default + `AUTH_BCRYPT_COST` operator knob + rehash-on-login + timing-oracle dummy in `pkg/auth/dummy.go` (T-8) | 2.4.1, 2.4.4 | 2026-05-28 |
| `Cache-Control: no-store` on `/api/{auth,profile,users,tokens}/*` (T-1 residual) | 8.1.2, 8.2.1 | 2026-05-28 |
| File upload MIME / magic-byte validation via `filemanagermime.Checker` (C-8 / T-9) | 12.4.1 | 2026-05-28 |
| `govulncheck` weekly + on-push CI workflow with auto-issue/auto-close (T-10) | 14.1.3, 14.2.2 | 2026-05-28 |
| `SECURITY.md` top-level vulnerability-disclosure policy | 14.2.6 | 2026-05-28 |
| PAT revoke writes to denylist (`pat:<id>`) + middleware re-checks post-lookup | 3.3.1 | 2026-05-28 |
| Re-auth helper (`internal/api/base/VerifyCurrentPassword`) + audit events `auth.reauth.{success,failure}` | 2.1.6, 3.7.1 | 2026-05-28 (helper); handler integration deferred |

### Still open

| ID | Description | ASVS req | Severity |
| --- | --- | --- | --- |
| — | Admin-MFA *enforcement* — soft-nudge + hard-fail scaffolding shipped (`internal/services/mfanudge` + config); DB migration (`users.mfa_nudge_first_shown_at`, `mfa_nudge_snoozed_until`), repository methods, login-flow integration, `POST /api/profile/mfa-nudge/snooze` endpoint and frontend modal remain | 4.3.1 | Roadmap |
| — | Idle session timeout — `pkg/auth/idle_tracker.go` (`CacheIdleTracker`) + config shipped; middleware wiring (session-PASETO-only filter, post-credentials check + probabilistic refresh) and WebSocket ping bridge remain | 3.3.2 | Roadmap |
| — | Re-auth helper applied to PAT-create / user-update / role-assign / 2FA-disable handlers — helper + audit events + unit tests shipped, handler wiring deferred (17-case `posttoken/handler_test.go` rewrite) | 2.1.6, 3.7.1, 4.3.3 | Roadmap |
| — | Alerting on repeated AC denials (audit stream now exists; needs SIEM/alert pipeline) | 4.1.5 | Roadmap |
| — | SBOM (CycloneDX) generation in release pipeline | 14.1.5 | Roadmap |
| — | Cookie hardening when cookies are issued | 3.4.x | Roadmap |
| — | Anti-automation on write-heavy flows other than login | 11.1.4 (extended) | Roadmap |
| — | Threat-model document for new features | 1.1.2, 1.8.1 | Roadmap |

---

## 7. References

- OWASP ASVS 4.0.3: https://owasp.org/www-project-application-security-verification-standard/
- OWASP API Security Top 10:2023: https://owasp.org/API-Security/editions/2023/
- CWE Top 25: https://cwe.mitre.org/top25/
- Project security tests: `internal/api/router_security_*_test.go` and `internal/api/middlewares/*_test.go`
- Project security testing convention: see project memory note "Security tests OWASP labels" — every security test file must comment its OWASP API Top 10:2023 category in the file header and in each test function.
