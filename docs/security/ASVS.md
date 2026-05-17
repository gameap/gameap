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
| Primary trust boundaries | (a) public Internet → API HTTP server, (b) game daemon → API gRPC/HTTP, (c) operator → admin endpoints |
| Last reviewed | 2026-05-18 (re-audit: MFA/TOTP, captcha, audit logging, safe file serving landed since 2026-04-28) |
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
| API2 — Broken Authentication | V2 (Authentication), V3 (Session) | `router_security_auth_test.go`, `router_security_auth_fuzz_test.go`, `router_security_daemon_test.go` |
| API3 — Broken Object Property Level Authorization | V4 (Access Control) | `router_security_escalation_test.go`, `router_security_escalation_fuzz_test.go` |
| API4 — Unrestricted Resource Consumption | V11 (Business Logic), V12 (Files) | partially: `internal/api/filemanager/upload/handler_test.go` (size cap) |
| API5 — Broken Function Level Authorization | V4 (Access Control) | `router_security_escalation_test.go` |
| API6 — Unrestricted Access to Sensitive Business Flows | V11 (Business Logic) | _gap — see §6_ |
| API7 — Server-Side Request Forgery | V12 (Files), V13 (API) | covered by absence of user-supplied URLs in outbound calls |
| API8 — Security Misconfiguration | V14 (Configuration) | `router_security_daemon_test.go` (setup keys) |
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
| 1.4.1 | Trusted enforcement points (gateway, server-side) | ✅ Met | All authn/authz performed server-side via `internal/api/middlewares/auth.go`, `personal_access.go`, `daemon.go`, `cors.go`, `https_redirect.go`. |
| 1.4.4 | Single vetted access-control mechanism | ✅ Met | RBAC concentrated in `internal/rbac/rbac.go`; ability checks routed via `base.RBAC` interface. |
| 1.5.1 | Trust boundaries documented | 🟡 Partial | Boundaries listed in §1 above; richer diagram in `docs/PROJECT_STRUCTURE.md` and `docs/gameap_architecture.svg`. |
| 1.8.1 | Sensitive data classified | ❌ Not met | Roadmap: classify daemon credentials, API tokens, user PII. |

### V2 Authentication

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 2.1.1 | Passwords are at least 12 characters | ❌ Not met | `internal/api/auth/login/input.go` validates non-empty only. Roadmap. |
| 2.1.2 | No truncation; allow ≥ 64 chars | 🟡 Partial | bcrypt limits to 72 bytes (`pkg/auth/password.go`); no explicit clamp/error. |
| 2.1.3 | Allow Unicode and spaces | 🟡 Partial | No explicit denial; password used verbatim. |
| 2.1.7 | Reject breached / common passwords | ❌ Not met | Roadmap. |
| 2.1.9 | No composition rules ("must contain X") | ✅ Met | None imposed. |
| 2.2.1 | Anti-automation on credential test endpoints | ✅ Met | Two layers: `LoginRateLimitMiddleware` (`internal/api/middlewares/login_ratelimit.go`) caps failed `/api/auth/login` attempts at 20/IP and 5/username per 15 min (429); and a captcha (`internal/services/captcha/service.go` — reCAPTCHA v2/v3 / Turnstile) verified *before* the user store is touched (`internal/api/auth/login/handler.go:87-95`). Verified by `TestRouterSecurity_API2_LoginBruteForceProtection`, `TestLoginRateLimitMiddleware_*` (9 unit tests), `internal/services/captcha/service_test.go`. |
| 2.2.2 | Lockout / similar after failures | ✅ Met | Same rate-limit middleware (sliding-window TTL self-recovers); the 2FA-verify endpoint additionally enforces a per-challenge 5-attempt budget (`internal/api/auth/twofactorverify/handler.go:26`). |
| 2.2.3 | Notify users of significant security events | ❌ Not met | Roadmap. |
| 2.3.1 | System-generated initial passwords are random and changeable | ➖ N/A | No system-issued user passwords; admins set them. |
| 2.4.1 | Passwords stored using approved KDF | ✅ Met | bcrypt (`pkg/auth/password.go`). |
| 2.4.2 | Salt is random and unique | ✅ Met | bcrypt salt is per-hash. |
| 2.5.1 | Password recovery does not reveal stored hash | 🟡 Partial | No recovery flow yet. Once added, must comply. |
| 2.5.4 | No default credentials shipped | 🟡 Partial | Setup tokens are random or operator-supplied. Tests: `TestRouterSecurity_API8_DaemonSetupTokenValidation`, `TestRouterSecurity_API8_EnrollmentSetupKeyValidation` (`router_security_daemon_test.go`). |
| 2.5.5 | Forgot-password tokens are single-use, time-bound | ➖ N/A | No reset flow yet. |
| 2.6.x | Out-of-band authenticators (SMS, email link) | ➖ N/A | Not implemented. |
| 2.7.x | OTP / lookup secrets / TOTP | ✅ Met | RFC-6238 TOTP second factor with 10 single-use bcrypt-hashed recovery codes (lookup secrets). Secret AES-256-GCM-encrypted at rest, replay-locked last-used step, scope-confined `g2fa_` challenge token, per-challenge 5-attempt budget. `pkg/twofactor/`, `internal/api/auth/twofactorverify/handler.go`, `internal/api/auth/login/handler.go:139-216`, enrollment under `internal/api/profile/twofactor/`. Tests: `pkg/twofactor/twofactor_test.go`, `internal/api/middlewares/auth_twofactor_security_test.go`. (Opt-in per user; admin-MFA *enforcement* still on the roadmap — see 4.3.1.) |
| 2.8.1 | Single-factor cryptographic auth tied to a device | ✅ Met | Daemon mTLS path: `internal/grpc/interceptors/auth.go:101` (`verifyMTLS`). |
| 2.9.1 | Cryptographic key material protected | ✅ Met | PASETO key validated for length: `pkg/auth/paseto.go:18` (32-byte symmetric key). |
| 2.9.2 | Verifiers are stored as one-way values | ✅ Met | All bearer credentials are stored hashed: passwords via bcrypt (`pkg/auth/password.go`); PATs via SHA-256 with constant-time comparison (`internal/api/middlewares/auth.go:222` using `subtle.ConstantTimeCompare`); daemon API tokens via SHA-256 (`internal/api/daemonapi/gettoken/handler.go` writes the hash, `internal/api/middlewares/daemon.go` hashes presented `X-Auth-Token` before lookup). Migration `007_hash_daemon_api_tokens.go` upgrades existing rows in mysql/postgres/sqlite. Verified by `TestRouterSecurity_API2_PATSecretMustBeOpaque` and `TestRouterSecurity_API2_DaemonAPITokenStoredAsHash`. |
| 2.10.1 | Secrets ≥ 128 bits | ✅ Met | PAT secret 48 random bytes (`internal/api/tokens/posttoken/handler.go`); daemon token 64 chars (`internal/api/daemonapi/gettoken/handler.go:91`); PASETO local key 32 bytes. |

Authentication tests:
`TestRouterSecurity_API2_BrokenAuthentication`,
`TestRouterSecurity_API2_TokenSchemes`,
`TestRouterSecurity_API2_TokenViaQueryAndCookie`,
`TestRouterSecurity_API2_UserDeletedAfterTokenIssue`,
`TestRouterSecurity_API2_PATSecretMustBeOpaque`,
`TestRouterSecurity_API2_DaemonAPIAuth`,
`TestRouterSecurity_API2_DaemonAPITokenStoredAsHash`,
`TestRouterSecurity_API2_LoginBruteForceProtection`,
`TestRouterSecurity_API2_LogoutInvalidatesToken`,
`TestRouterSecurity_API2_LogoutRequiresAuth`,
fuzz: `FuzzAuthMiddleware_AuthorizationHeader`, `FuzzAuthMiddleware_TokenParsing`,
`FuzzAuthMiddleware_AdminEndpointBypass` — all in `internal/api/router_security_auth*_test.go` and `router_security_daemon_test.go`.

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
| 8.1.1 | No sensitive data in URLs | 🟡 Partial | `?token=` accepted for WebSocket compatibility (`internal/api/middlewares/auth.go:227-230`). A single-use ≤10 s `glst_` token (`pkg/auth/shortlived.go`, `internal/api/auth/shorttoken/handler.go`) is now the intended URL credential; documented operational risk remains because the query path still accepts long-lived tokens too. |
| 8.2.1 | Browser caching of sensitive responses controlled | 🟡 Partial | No explicit `Cache-Control` for auth responses. Roadmap. |
| 8.3.1 | Sensitive data masked in responses | ✅ Met | Daemon API key, password and login fields removed from `nodeResponse` in `internal/api/nodes/{getnode,putnode}/response.go`; `getdaemonstatus/response.go` returns a `has_api_key` boolean instead of the key value. Daemon API tokens are stored as SHA-256 hashes (see 2.9.2). Verified by `TestRouterSecurity_API3_NodeResponseDoesNotLeakDaemonSecrets`. |
| 8.3.4 | Sensitive data classified and protected | ❌ Not met | Roadmap (V1.8.1 too). |

### V9 Communications

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 9.1.1 | TLS for all inbound and outbound traffic | 🟡 Partial | HTTPS redirect available: `internal/api/middlewares/https_redirect.go` + `https_redirect_test.go`. Enforcement is operator config. |
| 9.1.2 | Strong TLS configuration | 🟡 Partial | TLS config left to operator / reverse proxy. |
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
| 12.4.1 | Uploaded file content type and signature validated | 🟡 Partial | Filename + size only; magic-byte check is a gap. |
| 12.5.1 | Uploaded files served from a different domain or with safe headers | ✅ Met | `SafeContentHeaders` (`internal/api/filemanager/filemanagerhttp/headers.go`) sets `X-Content-Type-Options: nosniff` + `Content-Security-Policy: sandbox` on every served file, serves only an inert-MIME allowlist `inline` (SVG excluded), forces everything else to an opaque `attachment` (RFC 2231/6266). Test: `filemanagerhttp/headers_test.go`. |
| 12.6.1 | Untrusted SSRF blocked | ✅ Met | Outbound URLs come from configuration, not user input (`internal/services/globalapi.go`, `pluginstore/service.go`). |

### V13 API and Web Service

| # | Requirement (paraphrased) | Status | Evidence / Notes |
| --- | --- | --- | --- |
| 13.1.1 | API uses defined schema (OpenAPI/GraphQL) | ✅ Met | `openapi/openapi.yaml`. |
| 13.1.3 | API consumes only declared content types | ✅ Met | JSON enforced in handlers via decoder; alternate types rejected with 4xx. |
| 13.1.4 | Different processing paths for different content types | ✅ Met | `/api/*` vs `/gdaemon_api/*` use distinct middlewares (`internal/api/router.go`). |
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
| 14.1.3 | Dependencies up to date and patched | 🟡 Partial | `go.mod`/`go.sum` are version-pinned; no automated SCA pipeline tracked here. Roadmap. |
| 14.2.1 | Components are inventoried | 🟡 Partial | Implicit via `go.mod`. Roadmap: SBOM. |
| 14.2.5 | Default credentials removed | ✅ Met | Setup keys are operator-supplied or random; tested by `TestRouterSecurity_API8_DaemonSetupTokenValidation`, `TestRouterSecurity_API8_EnrollmentSetupKeyValidation`. |
| 14.3.1 | Debug features disabled in production | 🟡 Partial | Operator config. Roadmap: explicit production flag check. |
| 14.3.2 | Security HTTP headers (HSTS, X-Frame-Options, X-Content-Type-Options, CSP) | ❌ Not met | Roadmap. |
| 14.3.3 | Cross-Origin Resource Policy / referrer policy | ❌ Not met | Roadmap. |
| 14.4.1 | Every response has Content-Type | ✅ Met | Set by `pkg/api/responder.go`. |
| 14.4.2 | Each response specifies safe character set | ✅ Met | JSON encoder default UTF-8. |
| 14.4.3 | Content-Type allowlist applied | ✅ Met | Handlers reject unexpected types. |
| 14.4.4 | CORS allow-list scoped to trusted domains | ✅ Met | `internal/api/middlewares/cors.go::deriveDefaultOrigin` picks the scheme from `TLS.ForceHTTPS` (no longer hardcoded to `http://`); operators can override the auto-derived origin via the `HTTP_ALLOWED_ORIGINS` env var. Verified by `TestNewCORSMiddleware_HTTPSWhenForceHTTPS`, `_RejectsHTTPOriginWhenForceHTTPS`, `_ExplicitAllowedOriginsWinsOverAutoDerived` in `internal/api/middlewares/cors_test.go`. |
| 14.4.5 | HTTP methods restricted | ✅ Met | Verb routing; CORS preflight handled by `rs/cors`. |
| 14.4.6 | Anti-clickjacking | ❌ Not met | Roadmap (X-Frame-Options / CSP `frame-ancestors`). |
| 14.4.7 | `X-Content-Type-Options: nosniff` | ❌ Not met | Roadmap. |
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
| `internal/api/router_security_daemon_test.go` | API2:2023 + API8:2023 | 7 (+ daemon-token-hash regression) | — |
| `internal/api/router_security_escalation_test.go` | API3:2023 + API5:2023 | 10 (+ daemon-secret-leak regression) | — |
| `internal/api/router_security_escalation_fuzz_test.go` | API3:2023 + API5:2023 | — | 3 |
| `internal/api/router_security_test.go` | API1/API5 token-ability + admin gating | 2 | — |
| `internal/api/router_security_helpers_test.go` | shared fixtures | (helpers) | — |

Middleware-level unit tests:

- `internal/api/middlewares/auth_test.go`
- `internal/api/middlewares/personal_access_test.go`
- `internal/api/middlewares/daemon_test.go` (uses SHA-256 hashes in fixtures)
- `internal/api/middlewares/cors_test.go` (incl. HTTPS scheme + explicit allow-list cases)
- `internal/api/middlewares/https_redirect_test.go`
- `internal/api/middlewares/daemon_grpc_guard_test.go`
- `internal/api/middlewares/recovery_test.go`
- `internal/api/middlewares/login_ratelimit_test.go` (9 cases: per-IP / per-username / reset-on-success / X-Real-IP / etc.)

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

### Still open

| ID | Description | ASVS req | Severity |
| --- | --- | --- | --- |
| — | Admin-MFA *enforcement* flag (`require_mfa_for_admins`); MFA capability itself is done | 4.3.1 | Roadmap |
| — | Alerting on repeated AC denials (audit stream now exists; needs SIEM/alert pipeline) | 4.1.5 | Roadmap |
| — | Security HTTP headers (HSTS, CSP, X-Content-Type-Options) — note: file-download responses already set `nosniff`/CSP `sandbox`; global API/SPA still uncovered | 14.3.2, 14.4.6, 14.4.7 | Roadmap |
| — | Idle session timeout | 3.3.2 | Roadmap |
| — | File upload magic-byte validation | 12.4.1 | Roadmap |
| — | Cookie hardening when cookies are issued | 3.4.x | Roadmap |
| — | Password policy (min length, breached-password check) | 2.1.x | Roadmap |
| — | Anti-automation on write-heavy flows other than login | 11.1.4 (extended) | Roadmap |
| — | Re-authentication for sensitive admin actions | 3.7.1 | Roadmap |
| — | Threat-model document for new features | 1.1.2, 1.8.1 | Roadmap |

---

## 7. References

- OWASP ASVS 4.0.3: https://owasp.org/www-project-application-security-verification-standard/
- OWASP API Security Top 10:2023: https://owasp.org/API-Security/editions/2023/
- CWE Top 25: https://cwe.mitre.org/top25/
- Project security tests: `internal/api/router_security_*_test.go` and `internal/api/middlewares/*_test.go`
- Project security testing convention: see project memory note "Security tests OWASP labels" — every security test file must comment its OWASP API Top 10:2023 category in the file header and in each test function.
