package auth

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

// Blocklist exposes a membership test for known-weak passwords. Implementations
// MUST treat the candidate as already-lowercased — the caller (ValidatePassword)
// applies strings.ToLower once before the lookup.
type Blocklist interface {
	Contains(lowered string) bool
	Len() int
}

// NoopBlocklist is the default implementation: it has no entries and rejects
// nothing. Used when the embedded asset cannot be loaded, when the operator
// sets AUTH_ALLOW_WEAK_PASSWORDS=true, or when test code imports pkg/auth
// without going through the application bootstrap.
type NoopBlocklist struct{}

func (NoopBlocklist) Contains(string) bool { return false }
func (NoopBlocklist) Len() int             { return 0 }

// MapBlocklist holds the loaded common-passwords corpus. The map keys are
// always lowercase — callers MUST pre-lowercase the candidate.
type MapBlocklist struct {
	entries map[string]struct{}
}

func (m *MapBlocklist) Contains(lowered string) bool {
	if m == nil || m.entries == nil {
		return false
	}

	_, ok := m.entries[lowered]

	return ok
}

func (m *MapBlocklist) Len() int {
	if m == nil {
		return 0
	}

	return len(m.entries)
}

// ErrBlocklistEmpty is returned when the parsed source contains no entries
// after the gzip pass. The Container treats this as a load failure and
// falls back to NoopBlocklist with a slog.Error log.
var ErrBlocklistEmpty = errors.New("password blocklist is empty after parse")

// LoadEmbeddedBlocklist decompresses the //go:embed-loaded common-passwords
// asset (see blocklist_data.go) and returns a queryable MapBlocklist.
func LoadEmbeddedBlocklist() (Blocklist, error) {
	return LoadBlocklistFromBytes(commonPasswordsGz)
}

// LoadBlocklistFromBytes parses a gzipped, newline-separated list of
// lowercased passwords. Empty lines and stray whitespace are ignored.
func LoadBlocklistFromBytes(gz []byte) (Blocklist, error) {
	if len(gz) == 0 {
		return nil, ErrBlocklistEmpty
	}

	reader, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, errors.Wrap(err, "failed to open gzip reader")
	}

	defer func() { _ = reader.Close() }()

	raw, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decompress blocklist")
	}

	entries := make(map[string]struct{}, 1<<17)

	for line := range strings.SplitSeq(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		entries[trimmed] = struct{}{}
	}

	if len(entries) == 0 {
		return nil, ErrBlocklistEmpty
	}

	return &MapBlocklist{entries: entries}, nil
}

var (
	policyMu           sync.RWMutex
	passwordBlocklist  Blocklist = NoopBlocklist{}
	allowWeakPasswords bool
)

// SetPasswordBlocklist installs the package-wide blocklist used by
// ValidatePassword. Safe to call from concurrent goroutines; in practice it
// is called once during application bootstrap before the HTTP listener binds.
// Passing nil resets to NoopBlocklist.
func SetPasswordBlocklist(b Blocklist) {
	policyMu.Lock()
	defer policyMu.Unlock()

	if b == nil {
		passwordBlocklist = NoopBlocklist{}

		return
	}

	passwordBlocklist = b
}

// SetAllowWeakPasswords toggles the operator override. When true,
// ValidatePassword skips the blocklist lookup (length checks still run).
func SetAllowWeakPasswords(allow bool) {
	policyMu.Lock()
	defer policyMu.Unlock()

	allowWeakPasswords = allow
}

// PasswordBlocklist returns the currently installed blocklist. Useful for
// tests that want to probe the loaded asset directly.
func PasswordBlocklist() Blocklist {
	policyMu.RLock()
	defer policyMu.RUnlock()

	return passwordBlocklist
}

// AllowWeakPasswords reports the current operator override flag.
func AllowWeakPasswords() bool {
	policyMu.RLock()
	defer policyMu.RUnlock()

	return allowWeakPasswords
}

// ResetPasswordPolicy restores the package-level policy state to its
// zero-value defaults (NoopBlocklist + AllowWeak=false). Tests should call
// this from t.Cleanup to prevent cross-test leakage.
func ResetPasswordPolicy() {
	policyMu.Lock()
	defer policyMu.Unlock()

	passwordBlocklist = NoopBlocklist{}
	allowWeakPasswords = false
}
