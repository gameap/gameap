package auth

import _ "embed"

// commonPasswordsGz is the gzipped, lowercased, deduplicated, sorted list of
// known weak passwords. It is consulted by ValidatePassword via
// LoadEmbeddedBlocklist to reject pre-breached entries (OWASP ASVS 4.0.3
// §2.1.7).
//
// Source: SecLists Common-Credentials, MIT-licensed. See
// https://github.com/danielmiessler/SecLists under
// Passwords/Common-Credentials/xato-net-10-million-passwords-1000000.txt
//
// The asset is committed to the repo; rebuild instructions live in
// `pkg/auth/data/passwords/README.md`.
//
// Entries shorter than auth.MinPasswordLength are filtered out at build
// time because such inputs are already rejected by the length check.
//
//go:embed data/passwords/common-passwords.txt.gz
var commonPasswordsGz []byte
