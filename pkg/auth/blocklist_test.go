package auth

import (
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoopBlocklist_AlwaysFalse(t *testing.T) {
	t.Parallel()

	b := NoopBlocklist{}

	assert.False(t, b.Contains(""))
	assert.False(t, b.Contains("password1234"))
	assert.Equal(t, 0, b.Len())
}

func TestMapBlocklist_Contains(t *testing.T) {
	t.Parallel()

	b := &MapBlocklist{entries: map[string]struct{}{
		"password1234":   {},
		"qwertyuiop12":   {},
		"correctbattery": {},
	}}

	assert.True(t, b.Contains("password1234"))
	assert.True(t, b.Contains("qwertyuiop12"))
	assert.False(t, b.Contains("Password1234"), "Contains must be case-sensitive — caller pre-lowercases")
	assert.False(t, b.Contains("not_in_list"))
	assert.False(t, b.Contains(""))
	assert.Equal(t, 3, b.Len())
}

func TestMapBlocklist_NilSafe(t *testing.T) {
	t.Parallel()

	var b *MapBlocklist

	assert.False(t, b.Contains("anything"))
	assert.Equal(t, 0, b.Len())
}

func TestLoadBlocklistFromBytes_RoundTrip(t *testing.T) {
	t.Parallel()

	gz := gzipBytes(t, "password1234\nqwertyuiop12\ncorrectbattery\n")

	bl, err := LoadBlocklistFromBytes(gz)
	require.NoError(t, err)

	assert.Equal(t, 3, bl.Len())
	assert.True(t, bl.Contains("password1234"))
	assert.True(t, bl.Contains("qwertyuiop12"))
	assert.True(t, bl.Contains("correctbattery"))
	assert.False(t, bl.Contains("not_in_list"))
}

func TestLoadBlocklistFromBytes_IgnoresEmptyAndWhitespace(t *testing.T) {
	t.Parallel()

	gz := gzipBytes(t, "\n\npassword1234\n  qwertyuiop12  \n\n\n")

	bl, err := LoadBlocklistFromBytes(gz)
	require.NoError(t, err)

	assert.Equal(t, 2, bl.Len())
	assert.True(t, bl.Contains("password1234"))
	assert.True(t, bl.Contains("qwertyuiop12"))
}

func TestLoadBlocklistFromBytes_RejectsEmptyInput(t *testing.T) {
	t.Parallel()

	_, err := LoadBlocklistFromBytes(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBlocklistEmpty)
}

func TestLoadBlocklistFromBytes_RejectsEmptyAfterParse(t *testing.T) {
	t.Parallel()

	gz := gzipBytes(t, "")

	_, err := LoadBlocklistFromBytes(gz)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBlocklistEmpty)
}

func TestLoadBlocklistFromBytes_RejectsCorruptGzip(t *testing.T) {
	t.Parallel()

	_, err := LoadBlocklistFromBytes([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrBlocklistEmpty)
	assert.Contains(t, err.Error(), "gzip")
}

//nolint:paralleltest // mutates the package-level password policy (SetPasswordBlocklist) shared with other tests
func TestSetPasswordBlocklist_NilFallsBackToNoop(t *testing.T) {
	t.Cleanup(ResetPasswordPolicy)

	custom := &MapBlocklist{entries: map[string]struct{}{"x": {}}}
	SetPasswordBlocklist(custom)
	assert.Equal(t, 1, PasswordBlocklist().Len())

	SetPasswordBlocklist(nil)
	bl := PasswordBlocklist()
	_, isNoop := bl.(NoopBlocklist)
	assert.True(t, isNoop, "nil should reset to NoopBlocklist")
}

//nolint:paralleltest // mutates the package-level password policy (SetAllowWeakPasswords) shared with other tests
func TestSetAllowWeakPasswords_Toggles(t *testing.T) {
	t.Cleanup(ResetPasswordPolicy)

	assert.False(t, AllowWeakPasswords())

	SetAllowWeakPasswords(true)
	assert.True(t, AllowWeakPasswords())

	SetAllowWeakPasswords(false)
	assert.False(t, AllowWeakPasswords())
}

//nolint:paralleltest // mutates the package-level password policy (ResetPasswordPolicy) shared with other tests
func TestResetPasswordPolicy_RestoresDefaults(t *testing.T) {
	SetPasswordBlocklist(&MapBlocklist{entries: map[string]struct{}{"x": {}}})
	SetAllowWeakPasswords(true)

	ResetPasswordPolicy()

	assert.False(t, AllowWeakPasswords())

	bl := PasswordBlocklist()
	_, isNoop := bl.(NoopBlocklist)
	assert.True(t, isNoop)
	assert.Equal(t, 0, bl.Len())
}

func TestLoadEmbeddedBlocklist_HasReasonableCorpus(t *testing.T) {
	t.Parallel()

	bl, err := LoadEmbeddedBlocklist()
	require.NoError(t, err)

	assert.Greater(t, bl.Len(), 1000, "embedded asset should contain a non-trivial number of passwords")
}

func gzipBytes(t *testing.T, s string) []byte {
	t.Helper()

	var buf bytes.Buffer

	w := gzip.NewWriter(&buf)
	_, err := w.Write([]byte(s))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	return buf.Bytes()
}
