# Common-password blocklist

This directory holds the embedded common-password blocklist consulted by
`auth.ValidatePassword` to satisfy [OWASP ASVS 4.0.3
§2.1.7](https://github.com/OWASP/ASVS/) ("Verify that passwords submitted
during account registration, login, and password change are checked against
a set of breached passwords").

## Files

- `common-passwords.txt.gz` — gzipped, newline-separated list of lowercased
  passwords that are too common to allow. Loaded once at startup into a
  `map[string]struct{}` by `pkg/auth/blocklist.go`.

## Source

[SecLists](https://github.com/danielmiessler/SecLists), MIT-licensed —
specifically
`Passwords/Common-Credentials/xato-net-10-million-passwords-1000000.txt`
(the top-1M list derived from the xato.net 10M-password corpus).

## Filtering pipeline

The asset committed to the repo was produced from the raw SecLists file by:

1. Dropping entries shorter than `auth.MinPasswordLength` (12 bytes) — such
   inputs are already rejected by the length check, so storing them in the
   blocklist would waste space.
2. Dropping entries longer than `auth.MaxPasswordLength` (128 bytes).
3. Lowercasing every entry — the runtime check normalizes user input with
   `strings.ToLower` so the blocklist is logically case-insensitive.
4. Deduplicating.
5. Sorting lexicographically (deterministic output; git diffs remain
   meaningful once the .gz is regenerated).
6. Gzipping with `BestCompression`.

## Rebuilding

The asset is committed to the repo, so most operators never rebuild it.
When you do need to refresh the snapshot (e.g. SecLists updates), run this
one-liner from the project root — it reproduces the original filter
pipeline using standard POSIX tools:

```bash
curl -sL \
  https://raw.githubusercontent.com/danielmiessler/SecLists/master/Passwords/Common-Credentials/xato-net-10-million-passwords-1000000.txt \
  | awk 'length($0) >= 12 && length($0) <= 128 { print tolower($0) }' \
  | LC_ALL=C sort -u \
  | gzip -9 \
  > pkg/auth/data/passwords/common-passwords.txt.gz
```

After rebuilding, run `go test ./pkg/auth/... ./internal/api/...` to make
sure the corpus still contains the sentinel password used by
`internal/api/router_security_password_policy_test.go`.

## Why offline (not HIBP k-anonymity)

HIBP's k-anonymity API requires per-password outbound HTTPS calls to
`api.pwnedpasswords.com`. For air-gapped or restricted-egress GameAP
deployments that is a non-starter. The embedded list is a strict subset of
HIBP's coverage (top-1M, not the full breach corpus) but ships with zero
operational dependencies.

## Operator override

Setting `AUTH_ALLOW_WEAK_PASSWORDS=true` disables this check at the cost of
a startup `slog.Warn` and weakens compliance with ASVS §2.1.7. Use only
when explicitly required.
