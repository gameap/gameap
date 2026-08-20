package storage_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/acme"
	"github.com/gameap/gameap/internal/acme/storage"
	"github.com/gameap/gameap/internal/files"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingFileManager is a thin wrapper that lets a test inject IO errors at
// specific call sites without disturbing the InMemoryFileManager fast paths.
// It satisfies files.FileManager.
type failingFileManager struct {
	inner    *files.InMemoryFileManager
	readErr  error
	writeErr error
	delErr   error
}

func newFailingFM() *failingFileManager {
	return &failingFileManager{inner: files.NewInMemoryFileManager()}
}

func (fm *failingFileManager) Read(ctx context.Context, path string) ([]byte, error) {
	if fm.readErr != nil {
		return nil, fm.readErr
	}

	return fm.inner.Read(ctx, path)
}

func (fm *failingFileManager) Write(ctx context.Context, path string, data []byte) error {
	if fm.writeErr != nil {
		return fm.writeErr
	}

	return fm.inner.Write(ctx, path, data)
}

func (fm *failingFileManager) Delete(ctx context.Context, path string) error {
	if fm.delErr != nil {
		return fm.delErr
	}

	return fm.inner.Delete(ctx, path)
}

func (fm *failingFileManager) Exists(ctx context.Context, path string) bool {
	return fm.inner.Exists(ctx, path)
}

func (fm *failingFileManager) List(ctx context.Context, dir string) ([]string, error) {
	return fm.inner.List(ctx, dir)
}

func TestFileStorage_Account(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		account   *acme.Account
		wantError string
	}{
		{
			name: "regular_email",
			account: &acme.Account{
				Email:        "ops@example.com",
				PrivateKey:   []byte("PEM-DATA"),
				Registration: []byte(`{"uri":"https://acme/reg/1"}`),
			},
		},
		{
			name: "email_with_special_chars",
			account: &acme.Account{
				Email:        "ops+letsencrypt@example.com",
				PrivateKey:   []byte("KEY"),
				Registration: []byte(`{}`),
			},
		},
		{
			name:      "empty_email_rejected",
			account:   &acme.Account{Email: ""},
			wantError: "email is empty",
		},
		{
			name:      "nil_account_rejected",
			account:   nil,
			wantError: "account or email is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fm := files.NewInMemoryFileManager()
			s := storage.NewFileStorage(fm, "acme")

			err := s.SaveAccount(context.Background(), tt.account)
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)

			has, err := s.HasAccount(context.Background(), tt.account.Email)
			require.NoError(t, err)
			assert.True(t, has)

			loaded, err := s.LoadAccount(context.Background(), tt.account.Email)
			require.NoError(t, err)
			assert.Equal(t, tt.account.Email, loaded.Email)
			assert.Equal(t, tt.account.PrivateKey, loaded.PrivateKey)
			assert.Equal(t, []byte(tt.account.Registration), []byte(loaded.Registration))
		})
	}
}

func TestFileStorage_Resource(t *testing.T) {
	t.Parallel()

	fm := files.NewInMemoryFileManager()
	s := storage.NewFileStorage(fm, "acme")
	ctx := context.Background()

	resource := &certificate.Resource{
		Domain:            "*.example.com",
		CertURL:           "https://acme/cert/1",
		CertStableURL:     "https://acme/cert/stable/1",
		PrivateKey:        []byte("PRIVATE-KEY-PEM"),
		Certificate:       []byte("CERT-PEM"),
		IssuerCertificate: []byte("ISSUER-PEM"),
	}

	t.Run("save_load_round_trip", func(t *testing.T) {
		t.Parallel()

		err := s.SaveResource(ctx, "*.example.com,example.com", resource)
		require.NoError(t, err)

		has, err := s.HasResource(ctx, "*.example.com,example.com")
		require.NoError(t, err)
		assert.True(t, has)

		loaded, err := s.LoadResource(ctx, "*.example.com,example.com")
		require.NoError(t, err)
		assert.Equal(t, resource.Domain, loaded.Domain)
		assert.Equal(t, resource.CertURL, loaded.CertURL)
		assert.Equal(t, resource.PrivateKey, loaded.PrivateKey)
		assert.Equal(t, resource.Certificate, loaded.Certificate)
		assert.Equal(t, resource.IssuerCertificate, loaded.IssuerCertificate)
	})

	t.Run("delete_removes_resource", func(t *testing.T) {
		t.Parallel()

		err := s.SaveResource(ctx, "to-delete", resource)
		require.NoError(t, err)

		err = s.DeleteResource(ctx, "to-delete")
		require.NoError(t, err)

		has, err := s.HasResource(ctx, "to-delete")
		require.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("missing_resource_returns_error", func(t *testing.T) {
		t.Parallel()

		_, err := s.LoadResource(ctx, "non-existent")
		assert.Error(t, err)
	})

	t.Run("nil_resource_rejected", func(t *testing.T) {
		t.Parallel()

		err := s.SaveResource(ctx, "key", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resource is nil")
	})
}

func TestFileStorage_KeySanitization(t *testing.T) {
	t.Parallel()

	fm := files.NewInMemoryFileManager()
	s := storage.NewFileStorage(fm, "acme")

	resource := &certificate.Resource{
		Domain:      "*.example.com",
		Certificate: []byte("CERT"),
		PrivateKey:  []byte("KEY"),
	}

	keyWithWildcard := "*.example.com,example.com"
	err := s.SaveResource(context.Background(), keyWithWildcard, resource)
	require.NoError(t, err)

	loaded, err := s.LoadResource(context.Background(), keyWithWildcard)
	require.NoError(t, err)
	assert.Equal(t, resource.Domain, loaded.Domain)
}

// =============================================================================
// Error-injection tests (Tier 3 F)
// =============================================================================

func TestFileStorage_LoadAccount_ReturnsErrorOnCorruptedJSON(t *testing.T) {
	t.Parallel()

	// ARRANGE: bypass SaveAccount to plant invalid JSON at the same path
	// FileStorage will try to read.
	fm := files.NewInMemoryFileManager()
	require.NoError(t, fm.Write(context.Background(),
		"acme/accounts/ops_at_example.com.json", []byte("{not valid json")))

	s := storage.NewFileStorage(fm, "acme")

	// ACT
	loaded, err := s.LoadAccount(context.Background(), "ops@example.com")

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal account",
		"unmarshal error must be wrapped with helpful prefix")
	assert.Nil(t, loaded)
}

func TestFileStorage_LoadResource_ReturnsErrorWhenUnderlyingReadFails(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ioErr := errors.New("simulated disk read failure")
	fm := newFailingFM()
	require.NoError(t, fm.inner.Write(context.Background(),
		"acme/certificates/example.com.json", []byte("{}")))
	fm.readErr = ioErr

	s := storage.NewFileStorage(fm, "acme")

	// ACT
	loaded, err := s.LoadResource(context.Background(), "example.com")

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read resource",
		"underlying read error must be wrapped")
	assert.Contains(t, err.Error(), "simulated disk read failure",
		"original error message must propagate")
	assert.Nil(t, loaded)
}

func TestFileStorage_SaveAccount_ReturnsErrorOnWriteFailure(t *testing.T) {
	t.Parallel()

	// ARRANGE
	writeErr := errors.New("simulated write failure")
	fm := newFailingFM()
	fm.writeErr = writeErr

	s := storage.NewFileStorage(fm, "acme")

	account := &acme.Account{
		Email:        "ops@example.com",
		PrivateKey:   []byte("KEY"),
		Registration: []byte(`{}`),
	}

	// ACT
	err := s.SaveAccount(context.Background(), account)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write account",
		"write error must be wrapped")
	assert.Contains(t, err.Error(), "simulated write failure")
}

func TestFileStorage_SaveResource_ReturnsErrorOnWriteFailure(t *testing.T) {
	t.Parallel()

	// ARRANGE
	writeErr := errors.New("io write failed")
	fm := newFailingFM()
	fm.writeErr = writeErr

	s := storage.NewFileStorage(fm, "acme")

	// ACT
	err := s.SaveResource(context.Background(), "example.com", &certificate.Resource{
		Domain:      "example.com",
		PrivateKey:  []byte("KEY"),
		Certificate: []byte("CERT"),
	})

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write resource")
	assert.Contains(t, err.Error(), "io write failed")
}

func TestFileStorage_DeleteResource_ReturnsErrorWhenUnderlyingDeleteFails(t *testing.T) {
	t.Parallel()

	// ARRANGE
	delErr := errors.New("io delete failed")
	fm := newFailingFM()
	fm.delErr = delErr

	s := storage.NewFileStorage(fm, "acme")

	// ACT
	err := s.DeleteResource(context.Background(), "example.com")

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete resource")
}

func TestFileStorage_DeleteResource_NoErrorOnMissingKey(t *testing.T) {
	t.Parallel()

	// ARRANGE: InMemoryFileManager.Delete is a silent no-op for missing paths.
	// FileStorage must therefore be idempotent on missing keys.
	fm := files.NewInMemoryFileManager()
	s := storage.NewFileStorage(fm, "acme")

	// ACT
	err := s.DeleteResource(context.Background(), "never-saved")

	// ASSERT
	require.NoError(t, err, "DeleteResource must be idempotent on missing key")
}

func TestFileStorage_HasAccount_ReturnsFalseForEmptyEmail(t *testing.T) {
	t.Parallel()

	// ARRANGE
	s := storage.NewFileStorage(files.NewInMemoryFileManager(), "acme")

	// ACT
	has, err := s.HasAccount(context.Background(), "")

	// ASSERT
	require.NoError(t, err, "HasAccount with empty email must not error")
	assert.False(t, has)
}

func TestFileStorage_LoadAccount_RejectsEmptyEmail(t *testing.T) {
	t.Parallel()

	// ARRANGE
	s := storage.NewFileStorage(files.NewInMemoryFileManager(), "acme")

	// ACT
	loaded, err := s.LoadAccount(context.Background(), "")

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "email is empty")
	assert.Nil(t, loaded)
}

func TestFileStorage_AccountSanitization_HandlesUppercaseAndDots(t *testing.T) {
	t.Parallel()

	// ARRANGE: sanitizer replaces "@" with "_at_" and ".." with "_". Emails
	// that round-trip via the chosen email key are persistable; the storage
	// does not lowercase, but the lookup key is whatever string the caller
	// passes in. Verify a round trip with mixed case + ".." sequences.
	tests := []struct {
		name  string
		email string
	}{
		{name: "uppercase", email: "OPS@Example.COM"},
		{name: "subaddressed", email: "ops+letsencrypt@example.com"},
		{name: "dot_dot_in_local_part", email: "a..b@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fm := files.NewInMemoryFileManager()
			s := storage.NewFileStorage(fm, "acme")

			account := &acme.Account{
				Email:        tt.email,
				PrivateKey:   []byte("KEY"),
				Registration: []byte(`{}`),
			}

			require.NoError(t, s.SaveAccount(context.Background(), account))

			has, err := s.HasAccount(context.Background(), tt.email)
			require.NoError(t, err)
			assert.True(t, has, "HasAccount must find an account stored under the same key")

			loaded, err := s.LoadAccount(context.Background(), tt.email)
			require.NoError(t, err)
			assert.Equal(t, tt.email, loaded.Email)

			// Confirm sanitization actually occurred for paths that contain "@".
			paths, err := fm.List(context.Background(), "acme/accounts/")
			require.NoError(t, err)
			require.Len(t, paths, 1)
			assert.False(t, strings.Contains(paths[0], "@"),
				"the persisted path must not contain '@' (sanitized to _at_)")
		})
	}
}
