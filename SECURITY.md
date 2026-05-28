# Security Policy

The GameAP project takes the security of its software seriously. This document
describes how to report vulnerabilities, which versions receive security fixes,
and what to expect after a report is filed.

## Supported versions

Only the latest minor release line receives security patches. Older minor
versions are best-effort; report nonetheless, but expect backports only when a
fix is trivially applicable.

| Version | Supported          |
| ------- | ------------------ |
| `main` (development) | :white_check_mark: |
| latest stable release | :white_check_mark: |
| previous stable release | :warning: (best-effort) |
| older versions | :x: |

If you operate a self-hosted deployment, upgrading to the latest stable release
is the first remediation we will ask about — please confirm you are on a
supported version before filing a report.

## Reporting a vulnerability

**Do not open a public GitHub issue for security vulnerabilities.** Public
issues are crawled by attackers as soon as they are filed.

Use one of these channels, in order of preference:

1. **GitHub Security Advisory** (preferred).
   <https://github.com/gameap/gameap/security/advisories/new> — creates a
   private advisory only visible to the maintainers and the reporter. This is
   the lowest-friction option and gives us a single place to coordinate the
   fix and CVE assignment.

2. **Encrypted email** to `security@gameap.com`.
   - PGP fingerprint: *to be published in the next release; until then please
     prefer the GitHub Security Advisory channel above.*
   - When PGP is published, encrypt the report and attach the encrypted blob
     to a short plaintext mail describing severity only.

Please include:

- A clear description of the vulnerability and the impact you can demonstrate.
- The affected version (binary `gameap --version` output) and deployment shape
  (PostgreSQL / MySQL / SQLite; behind a reverse proxy; running plugins).
- Step-by-step reproduction, ideally with a minimal config or
  `curl`/`grpcurl` invocation.
- If you have a proposed fix or mitigation, please share it — we are happy to
  credit external contributions.

## What to expect

| Stage | Target |
| --- | --- |
| Initial acknowledgement | **within 72 hours** of report |
| Initial assessment & severity | **within 14 days** of acknowledgement |
| Fix availability | depends on severity (see below) |
| Public disclosure | **within 90 days** of report, coordinated with reporter |

Severity targets for fix availability (calendar days from acknowledgement):

- **Critical** (remote unauthenticated RCE, auth bypass, full data
  exfiltration): patch released within 14 days.
- **High** (privilege escalation, IDOR / BOLA at scale, SSRF reaching
  cloud-metadata or internal networks): patch released within 30 days.
- **Medium** (authenticated abuse, info disclosure with limited blast
  radius): patch released within 60 days.
- **Low** (hardening, defence-in-depth gaps without an exploit path):
  rolled into the next regular release.

If we cannot meet the targets above we will explain why and propose a new
timeline. We will keep you informed throughout — no radio silence.

## Disclosure timeline

The default disclosure window is **90 days** from the initial report. We
follow the [Project Zero
model](https://googleprojectzero.blogspot.com/p/vulnerability-disclosure-faq.html):

- Fix and release coordinated with the reporter.
- After the fix ships and a reasonable patch-adoption window (typically 7
  days, longer for slow-moving deployments by mutual agreement), the
  advisory is published with reporter credit (unless anonymity is
  requested).
- Reporters who follow this process are credited in the release notes and
  the GitHub Security Advisory, and are eligible to be listed in a future
  `docs/security/HALL_OF_FAME.md`.

If a vulnerability is being actively exploited in the wild, we may reduce
the disclosure window and publish mitigation guidance ahead of the full fix.

## Scope

### In scope

- The `gameap-api` HTTP REST API, gRPC daemon control plane, embedded SPA,
  WebSocket endpoints.
- The plugin runtime (`internal/plugin/`) — sandbox escapes, SSRF via host
  libraries, host-library credential leaks.
- Authentication, session management, RBAC, audit-log integrity.
- File-manager paths under `internal/api/filemanager/`.
- Daemon enrollment flow and credentials at rest.
- Cryptographic choices (PASETO, bcrypt, mTLS, TOTP) — concrete attacks on
  the deployed scheme, not theoretical preferences.

### Out of scope

- Issues that require operator misconfiguration (e.g. running with
  `AUTH_ALLOW_WEAK_PASSWORDS=true`, exposing the daemon over a public network
  without TLS, disabling `SECURITY_HEADERS_ENABLED`). Operator hardening
  guidance belongs in documentation issues.
- Vulnerabilities in third-party dependencies that are already tracked in
  `govulncheck` output. Report a request to bump the dependency as a regular
  GitHub issue.
- Denial-of-service via raw resource consumption (e.g. flood a single
  reverse-proxied endpoint with traffic). Submit upstream as a feature
  request if the panel itself lacks a relevant cap.
- Social-engineering attacks against the operator (phishing, malicious
  plugin authors tricking an admin into installing a plugin) — but the
  *plugin-sandbox boundary* itself is in scope.
- Issues in deployments that have modified the source code or replaced
  vendored dependencies.

### Demo deployment

`demo.gameap.com` runs the latest stable release. Non-destructive testing
against the demo is welcome under the conditions above; do not attempt to
deny service, harvest data of other users, or break out of the
demo's plugin sandbox into the host. If in doubt, ask first.

## Coordinated disclosure with downstream consumers

GameAP is consumed by hosting providers and self-hosted operators. If a
vulnerability affects a known integration we coordinate with that downstream
before the public advisory window closes. If you are integrating GameAP into
a managed product and want to be on the pre-disclosure list, contact
`security@gameap.com`.

## Security-related discussions and hardening guidance

For hardening recommendations that are **not** vulnerabilities, please use:

- `docs/security/ASVS.md` and `docs/security/ASVS_L2.md` — the project's
  ASVS self-audit and known-gaps inventory.
- Regular GitHub issues with the `security-hardening` label.

This file is reserved for the disclosure process.
