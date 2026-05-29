package certificates

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net"
	"slices"
	"testing"

	"github.com/gameap/gameap/internal/files"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errInjected = errors.New("injected file manager error")

const (
	ensureCertPath = "test/ensure_cert.pem"
	ensureKeyPath  = "test/ensure_key.pem"
)

func TestService_Root(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(inner *files.InMemoryFileManager) files.FileManager
		wantError string
	}{
		{
			name: "root_certificate_does_not_exist_generates_new",
			setup: func(inner *files.InMemoryFileManager) files.FileManager {
				return inner
			},
		},
		{
			name: "read_error_when_certificate_exists",
			setup: func(_ *files.InMemoryFileManager) files.FileManager {
				return &files.MockFileManager{
					ExistsFunc: func(_ context.Context, _ string) bool {
						return true
					},
					ReadFunc: func(_ context.Context, _ string) ([]byte, error) {
						return nil, errInjected
					},
				}
			},
			wantError: "failed to read root certificate",
		},
		{
			name: "generate_error_when_write_returns_error",
			setup: func(inner *files.InMemoryFileManager) files.FileManager {
				return &files.MockFileManager{
					ExistsFunc: inner.Exists,
					ReadFunc:   inner.Read,
					WriteFunc: func(_ context.Context, _ string, _ []byte) error {
						return errInjected
					},
				}
			},
			wantError: "failed to generate root certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			fm := tt.setup(files.NewInMemoryFileManager())
			service := NewService(fm)
			ctx := context.Background()

			// ACT
			cert, err := service.Root(ctx)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")

				return
			}

			require.NoError(t, err)
			assert.Contains(t, cert, "BEGIN CERTIFICATE")
			assert.Contains(t, cert, "END CERTIFICATE")

			block, _ := pem.Decode([]byte(cert))
			require.NotNil(t, block)
			assert.Equal(t, "CERTIFICATE", block.Type)

			parsedCert, err := x509.ParseCertificate(block.Bytes)
			require.NoError(t, err)
			assert.True(t, parsedCert.IsCA, "root certificate must be a CA")
		})
	}
}

func TestService_RootKey(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(inner *files.InMemoryFileManager) files.FileManager
		wantError string
	}{
		{
			name: "root_key_does_not_exist_generates_new",
			setup: func(inner *files.InMemoryFileManager) files.FileManager {
				return inner
			},
		},
		{
			name: "read_error_when_key_exists",
			setup: func(_ *files.InMemoryFileManager) files.FileManager {
				return &files.MockFileManager{
					ExistsFunc: func(_ context.Context, _ string) bool {
						return true
					},
					ReadFunc: func(_ context.Context, _ string) ([]byte, error) {
						return nil, errInjected
					},
				}
			},
			wantError: "failed to read root key",
		},
		{
			name: "generate_error_when_write_returns_error",
			setup: func(inner *files.InMemoryFileManager) files.FileManager {
				return &files.MockFileManager{
					ExistsFunc: inner.Exists,
					ReadFunc:   inner.Read,
					WriteFunc: func(_ context.Context, _ string, _ []byte) error {
						return errInjected
					},
				}
			},
			wantError: "failed to generate root key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			fm := tt.setup(files.NewInMemoryFileManager())
			service := NewService(fm)
			ctx := context.Background()

			// ACT
			key, err := service.RootKey(ctx)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")

				return
			}

			require.NoError(t, err)
			assert.Contains(t, key, "BEGIN PRIVATE KEY")
			assert.Contains(t, key, "END PRIVATE KEY")

			block, _ := pem.Decode([]byte(key))
			require.NotNil(t, block)
			assert.Equal(t, "PRIVATE KEY", block.Type)
		})
	}
}

func TestService_Sign(t *testing.T) {
	tests := []struct {
		name string
		// setup returns the FileManager and the CSR to sign. An empty csrPEM
		// means a freshly generated valid CSR is used.
		setup     func(t *testing.T) (files.FileManager, string)
		opts      *SignOptions
		wantError string
		errType   error
	}{
		{
			name: "valid_csr_no_options",
			setup: func(t *testing.T) (files.FileManager, string) {
				t.Helper()

				return files.NewInMemoryFileManager(), generateTestCSR(t)
			},
		},
		{
			name: "valid_csr_with_options",
			setup: func(t *testing.T) (files.FileManager, string) {
				t.Helper()

				return files.NewInMemoryFileManager(), generateTestCSR(t)
			},
			opts: &SignOptions{
				CommonName:         "test.example.com",
				Email:              "test@example.com",
				Organization:       "Test Org",
				Country:            "US",
				State:              "CA",
				Locality:           "San Francisco",
				OrganizationalUnit: "IT",
			},
		},
		{
			name: "valid_csr_with_sans",
			setup: func(t *testing.T) (files.FileManager, string) {
				t.Helper()

				return files.NewInMemoryFileManager(), generateTestCSR(t)
			},
			opts: &SignOptions{
				IPAddresses: []net.IP{net.ParseIP("192.168.1.10"), net.ParseIP("10.0.0.1")},
				DNSNames:    []string{"sign.example.com", "alt.example.com"},
			},
		},
		{
			name: "invalid_csr_pem",
			setup: func(_ *testing.T) (files.FileManager, string) {
				return files.NewInMemoryFileManager(), "invalid pem"
			},
			wantError: "failed to parse CSR PEM",
			errType:   ErrFailedToParseCSRPEM,
		},
		{
			name: "malformed_csr",
			setup: func(_ *testing.T) (files.FileManager, string) {
				return files.NewInMemoryFileManager(),
					"-----BEGIN CERTIFICATE REQUEST-----\ninvalid\n-----END CERTIFICATE REQUEST-----"
			},
			wantError: "failed to parse CSR",
		},
		{
			name: "tampered_csr_signature",
			setup: func(t *testing.T) (files.FileManager, string) {
				t.Helper()

				return files.NewInMemoryFileManager(), tamperCSRSignature(t, generateTestCSR(t))
			},
			wantError: "failed to verify CSR signature",
		},
		{
			name: "garbage_root_certificate",
			setup: func(t *testing.T) (files.FileManager, string) {
				t.Helper()

				inner := files.NewInMemoryFileManager()
				require.NoError(t, inner.Write(context.Background(), RootCACert, []byte("garbage cert")))

				fm := &files.MockFileManager{
					ExistsFunc: inner.Exists,
					ReadFunc:   inner.Read,
					WriteFunc:  inner.Write,
				}

				return fm, generateTestCSR(t)
			},
			wantError: "failed to parse root certificate PEM",
			errType:   ErrFailedToParseRootCertPEM,
		},
		{
			name: "garbage_root_key",
			setup: func(t *testing.T) (files.FileManager, string) {
				t.Helper()

				inner := seedRootMaterial(t)

				fm := &files.MockFileManager{
					ExistsFunc: inner.Exists,
					WriteFunc:  inner.Write,
					ReadFunc: func(ctx context.Context, path string) ([]byte, error) {
						if path == RootCAKey {
							return []byte("garbage key"), nil
						}

						return inner.Read(ctx, path)
					},
				}

				return fm, generateTestCSR(t)
			},
			wantError: "failed to parse root key PEM",
			errType:   ErrFailedToParseRootKeyPEM,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			fm, csrPEM := tt.setup(t)
			service := NewService(fm)
			ctx := context.Background()

			// ACT
			certPEM, err := service.Sign(ctx, csrPEM, tt.opts)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}

				return
			}

			require.NoError(t, err)
			assert.Contains(t, certPEM, "BEGIN CERTIFICATE")
			assert.Contains(t, certPEM, "END CERTIFICATE")

			block, _ := pem.Decode([]byte(certPEM))
			require.NotNil(t, block)
			assert.Equal(t, "CERTIFICATE", block.Type)

			parsedCert, err := x509.ParseCertificate(block.Bytes)
			require.NoError(t, err)
			assert.False(t, parsedCert.IsCA, "signed leaf must not be a CA")

			if tt.opts != nil {
				validateSignOptions(t, parsedCert, tt.opts)
			}
		})
	}
}

func TestService_Generate(t *testing.T) {
	tests := []struct {
		name            string
		certificatePath string
		keyPath         string
		setup           func(inner *files.InMemoryFileManager, writes *[]string) files.FileManager
		opts            *SignOptions
		wantError       string
	}{
		{
			name:            "generate_with_default_options",
			certificatePath: "test/cert.pem",
			keyPath:         "test/key.pem",
			setup: func(inner *files.InMemoryFileManager, _ *[]string) files.FileManager {
				return inner
			},
		},
		{
			name:            "generate_with_custom_options",
			certificatePath: "test/custom_cert.pem",
			keyPath:         "test/custom_key.pem",
			setup: func(inner *files.InMemoryFileManager, _ *[]string) files.FileManager {
				return inner
			},
			opts: &SignOptions{
				CommonName:         "custom.example.com",
				Email:              "custom@example.com",
				Organization:       "Custom Org",
				Country:            "UK",
				State:              "London",
				Locality:           "Westminster",
				OrganizationalUnit: "Dev",
			},
		},
		{
			name:            "generate_with_sans",
			certificatePath: "test/san_cert.pem",
			keyPath:         "test/san_key.pem",
			setup: func(inner *files.InMemoryFileManager, _ *[]string) files.FileManager {
				return inner
			},
			opts: &SignOptions{
				IPAddresses: []net.IP{net.ParseIP("192.168.1.10")},
				DNSNames:    []string{"gen.example.com"},
			},
		},
		{
			name:            "certificate_write_error",
			certificatePath: "test/cert.pem",
			keyPath:         "test/key.pem",
			setup: func(inner *files.InMemoryFileManager, _ *[]string) files.FileManager {
				return &files.MockFileManager{
					ExistsFunc: inner.Exists,
					ReadFunc:   inner.Read,
					WriteFunc: func(ctx context.Context, path string, data []byte) error {
						if path == "test/cert.pem" {
							return errInjected
						}

						return inner.Write(ctx, path, data)
					},
				}
			},
			wantError: "failed to write certificate",
		},
		{
			name:            "private_key_write_error",
			certificatePath: "test/cert.pem",
			keyPath:         "test/key.pem",
			setup: func(inner *files.InMemoryFileManager, _ *[]string) files.FileManager {
				return &files.MockFileManager{
					ExistsFunc: inner.Exists,
					ReadFunc:   inner.Read,
					WriteFunc: func(ctx context.Context, path string, data []byte) error {
						if path == "test/key.pem" {
							return errInjected
						}

						return inner.Write(ctx, path, data)
					},
				}
			},
			wantError: "failed to write private key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			inner := files.NewInMemoryFileManager()
			var writes []string
			fm := tt.setup(inner, &writes)
			service := NewService(fm)
			ctx := context.Background()

			// ACT
			certPEM, keyPEM, err := service.Generate(ctx, tt.certificatePath, tt.keyPath, tt.opts)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")

				return
			}

			require.NoError(t, err)

			assert.Contains(t, certPEM, "BEGIN CERTIFICATE")
			assert.Contains(t, keyPEM, "BEGIN PRIVATE KEY")

			certBlock, _ := pem.Decode([]byte(certPEM))
			require.NotNil(t, certBlock)
			parsedCert, err := x509.ParseCertificate(certBlock.Bytes)
			require.NoError(t, err)
			assert.False(t, parsedCert.IsCA, "generated leaf must not be a CA")

			keyBlock, _ := pem.Decode([]byte(keyPEM))
			require.NotNil(t, keyBlock)
			_, err = x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
			require.NoError(t, err)

			assert.True(t, inner.Exists(ctx, tt.certificatePath), "certificate must be persisted")
			assert.True(t, inner.Exists(ctx, tt.keyPath), "private key must be persisted")

			savedCert, err := inner.Read(ctx, tt.certificatePath)
			require.NoError(t, err)
			assert.Equal(t, certPEM, string(savedCert), "persisted certificate must match returned PEM")

			savedKey, err := inner.Read(ctx, tt.keyPath)
			require.NoError(t, err)
			assert.Equal(t, keyPEM, string(savedKey), "persisted key must match returned PEM")

			if tt.opts != nil {
				validateSignOptions(t, parsedCert, tt.opts)
				validateSANs(t, parsedCert, tt.opts)
			}
		})
	}
}

func TestService_EnsureGenerated(t *testing.T) {
	tests := []struct {
		name string
		// setup configures the FileManager and returns it together with a
		// recorder of leaf certificate/key writes (root writes are excluded).
		setup        func(t *testing.T, inner *files.InMemoryFileManager, leafWrites *[]string) files.FileManager
		opts         *SignOptions
		wantError    string
		wantLeafGen  bool
		wantSANCheck *SignOptions
	}{
		{
			name: "both_files_absent_delegates_to_generate",
			setup: func(_ *testing.T, inner *files.InMemoryFileManager, leafWrites *[]string) files.FileManager {
				return recordingLeafWrites(inner, leafWrites)
			},
			wantLeafGen: true,
		},
		{
			name: "both_present_and_nil_opts_returns_existing",
			setup: func(t *testing.T, inner *files.InMemoryFileManager, leafWrites *[]string) files.FileManager {
				t.Helper()
				seedLeafPair(t, inner, nil)

				return recordingLeafWrites(inner, leafWrites)
			},
			wantLeafGen: false,
		},
		{
			name: "both_present_and_sans_match_returns_existing",
			setup: func(t *testing.T, inner *files.InMemoryFileManager, leafWrites *[]string) files.FileManager {
				t.Helper()
				seedLeafPair(t, inner, &SignOptions{
					IPAddresses: []net.IP{net.ParseIP("192.168.1.10")},
					DNSNames:    []string{"ensure.example.com"},
				})

				return recordingLeafWrites(inner, leafWrites)
			},
			opts: &SignOptions{
				IPAddresses: []net.IP{net.ParseIP("192.168.1.10")},
				DNSNames:    []string{"ensure.example.com"},
			},
			wantLeafGen: false,
		},
		{
			name: "both_present_and_sans_mismatch_regenerates",
			setup: func(t *testing.T, inner *files.InMemoryFileManager, leafWrites *[]string) files.FileManager {
				t.Helper()
				seedLeafPair(t, inner, &SignOptions{
					DNSNames: []string{"old.example.com"},
				})

				return recordingLeafWrites(inner, leafWrites)
			},
			opts: &SignOptions{
				IPAddresses: []net.IP{net.ParseIP("10.20.30.40")},
				DNSNames:    []string{"new.example.com"},
			},
			wantLeafGen: true,
			wantSANCheck: &SignOptions{
				IPAddresses: []net.IP{net.ParseIP("10.20.30.40")},
				DNSNames:    []string{"new.example.com"},
			},
		},
		{
			name: "only_certificate_present_delegates_to_generate",
			setup: func(t *testing.T, inner *files.InMemoryFileManager, leafWrites *[]string) files.FileManager {
				t.Helper()
				require.NoError(t, inner.Write(context.Background(), ensureCertPath, []byte("existing cert")))

				return recordingLeafWrites(inner, leafWrites)
			},
			wantLeafGen: true,
		},
		{
			name: "certificate_read_error",
			setup: func(_ *testing.T, inner *files.InMemoryFileManager, _ *[]string) files.FileManager {
				return &files.MockFileManager{
					ExistsFunc: func(_ context.Context, _ string) bool {
						return true
					},
					ReadFunc: func(_ context.Context, _ string) ([]byte, error) {
						return nil, errInjected
					},
					WriteFunc: inner.Write,
				}
			},
			wantError: "failed to read existing certificate",
		},
		{
			name: "private_key_read_error",
			setup: func(t *testing.T, inner *files.InMemoryFileManager, _ *[]string) files.FileManager {
				t.Helper()
				seedLeafPair(t, inner, nil)

				return &files.MockFileManager{
					ExistsFunc: inner.Exists,
					WriteFunc:  inner.Write,
					ReadFunc: func(ctx context.Context, path string) ([]byte, error) {
						if path == ensureKeyPath {
							return nil, errInjected
						}

						return inner.Read(ctx, path)
					},
				}
			},
			wantError: "failed to read existing private key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			inner := files.NewInMemoryFileManager()
			var leafWrites []string
			fm := tt.setup(t, inner, &leafWrites)
			service := NewService(fm)
			ctx := context.Background()

			// ACT
			certPEM, keyPEM, err := service.EnsureGenerated(ctx, ensureCertPath, ensureKeyPath, tt.opts)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, certPEM)
			assert.NotEmpty(t, keyPEM)

			assert.True(t, inner.Exists(ctx, ensureCertPath), "certificate must exist after EnsureGenerated")
			assert.True(t, inner.Exists(ctx, ensureKeyPath), "private key must exist after EnsureGenerated")

			certBlock, _ := pem.Decode([]byte(certPEM))
			require.NotNil(t, certBlock)
			parsedCert, err := x509.ParseCertificate(certBlock.Bytes)
			require.NoError(t, err)
			assert.False(t, parsedCert.IsCA, "leaf certificate must not be a CA")

			if tt.wantLeafGen {
				assert.NotEmpty(t, leafWrites, "expected a regeneration that writes the leaf certificate")
			} else {
				assert.Empty(t, leafWrites, "existing certificate must be reused without new leaf writes")
			}

			if tt.wantSANCheck != nil {
				validateSANs(t, parsedCert, tt.wantSANCheck)
			}
		})
	}
}

func TestService_GenerateInMemory(t *testing.T) {
	tests := []struct {
		name string
		opts *SignOptions
	}{
		{
			name: "default_options",
		},
		{
			name: "custom_options_with_sans",
			opts: &SignOptions{
				CommonName:         "memory.example.com",
				Email:              "memory@example.com",
				Organization:       "Memory Org",
				Country:            "DE",
				State:              "Berlin",
				Locality:           "Mitte",
				OrganizationalUnit: "Ops",
				IPAddresses:        []net.IP{net.ParseIP("172.16.0.5")},
				DNSNames:           []string{"mem.example.com"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			inner := files.NewInMemoryFileManager()
			var writes []string
			fm := &files.MockFileManager{
				ExistsFunc: inner.Exists,
				ReadFunc:   inner.Read,
				WriteFunc: func(ctx context.Context, path string, data []byte) error {
					writes = append(writes, path)

					return inner.Write(ctx, path, data)
				},
			}
			service := NewService(fm)
			ctx := context.Background()

			// ACT
			certPEM, keyPEM, err := service.GenerateInMemory(ctx, tt.opts)

			// ASSERT
			require.NoError(t, err)

			certBlock, _ := pem.Decode([]byte(certPEM))
			require.NotNil(t, certBlock)
			parsedCert, err := x509.ParseCertificate(certBlock.Bytes)
			require.NoError(t, err)
			assert.False(t, parsedCert.IsCA, "in-memory leaf must not be a CA")

			keyBlock, _ := pem.Decode([]byte(keyPEM))
			require.NotNil(t, keyBlock)
			parsedKeyAny, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
			require.NoError(t, err)

			rsaKey, ok := parsedKeyAny.(*rsa.PrivateKey)
			require.True(t, ok, "private key must be RSA")

			certPubKey, ok := parsedCert.PublicKey.(*rsa.PublicKey)
			require.True(t, ok, "certificate public key must be RSA")
			assert.Equal(t, 0, rsaKey.N.Cmp(certPubKey.N), "key modulus must match certificate")
			assert.Equal(t, rsaKey.E, certPubKey.E, "key exponent must match certificate")

			assert.ElementsMatch(t, []string{RootCACert, RootCAKey}, writes,
				"only the root CA must be persisted; the leaf must stay in memory")

			if tt.opts != nil {
				validateSignOptions(t, parsedCert, tt.opts)
				validateSANs(t, parsedCert, tt.opts)
			}
		})
	}
}

func TestService_Fingerprint(t *testing.T) {
	wellFormedPEMBadCert := string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("this is valid base64 but not a DER certificate"),
	}))

	tests := []struct {
		name      string
		certPEM   string
		useReal   bool
		wantError string
		errType   error
	}{
		{
			name:    "valid_certificate",
			useReal: true,
		},
		{
			name:      "invalid_pem",
			certPEM:   "invalid pem",
			wantError: "failed to parse CSR PEM",
			errType:   ErrFailedToParseCSRPEM,
		},
		{
			name:      "undecodable_pem_body",
			certPEM:   "-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----",
			wantError: "failed to parse CSR PEM",
			errType:   ErrFailedToParseCSRPEM,
		},
		{
			name:      "well_formed_pem_with_invalid_certificate",
			certPEM:   wellFormedPEMBadCert,
			wantError: "failed to parse certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			fm := files.NewInMemoryFileManager()
			service := NewService(fm)
			ctx := context.Background()

			certPEM := tt.certPEM
			if tt.useReal {
				cert, _, err := service.Generate(ctx, "test/cert.pem", "test/key.pem", nil)
				require.NoError(t, err)
				certPEM = cert
			}

			// ACT
			fingerprint, err := service.Fingerprint(certPEM)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}

				return
			}

			require.NoError(t, err)
			require.Len(t, fingerprint, 64)
			assert.Regexp(t, "^[a-f0-9]{64}$", fingerprint)
		})
	}
}

func TestService_Fingerprint_Consistency(t *testing.T) {
	// ARRANGE
	fm := files.NewInMemoryFileManager()
	service := NewService(fm)
	ctx := context.Background()

	cert, _, err := service.Generate(ctx, "test/cert.pem", "test/key.pem", nil)
	require.NoError(t, err)

	// ACT
	fp1, err := service.Fingerprint(cert)
	require.NoError(t, err)

	fp2, err := service.Fingerprint(cert)
	require.NoError(t, err)

	// ASSERT
	assert.Equal(t, fp1, fp2, "fingerprint of the same certificate must be deterministic")
}

func TestService_RootGeneration_Persistence(t *testing.T) {
	// ARRANGE
	fm := files.NewInMemoryFileManager()
	service := NewService(fm)
	ctx := context.Background()

	// ACT
	cert1, err := service.Root(ctx)
	require.NoError(t, err)

	key1, err := service.RootKey(ctx)
	require.NoError(t, err)

	cert2, err := service.Root(ctx)
	require.NoError(t, err)

	key2, err := service.RootKey(ctx)
	require.NoError(t, err)

	// ASSERT
	assert.Equal(t, cert1, cert2, "root certificate must be generated only once")
	assert.Equal(t, key1, key2, "root key must be generated only once")
}

func TestCertificateMatchesSANs(t *testing.T) {
	withSANsCertPEM := func(t *testing.T, opts *SignOptions) []byte {
		t.Helper()

		service := NewService(files.NewInMemoryFileManager())
		certPEM, _, err := service.GenerateInMemory(context.Background(), opts)
		require.NoError(t, err)

		return []byte(certPEM)
	}

	tests := []struct {
		name    string
		certPEM func(t *testing.T) []byte
		opts    *SignOptions
		want    bool
	}{
		{
			name: "nil_opts_matches",
			certPEM: func(t *testing.T) []byte {
				t.Helper()

				return withSANsCertPEM(t, nil)
			},
			opts: nil,
			want: true,
		},
		{
			name: "empty_ip_and_dns_matches",
			certPEM: func(t *testing.T) []byte {
				t.Helper()

				return withSANsCertPEM(t, nil)
			},
			opts: &SignOptions{},
			want: true,
		},
		{
			name: "all_requested_ips_present_matches",
			certPEM: func(t *testing.T) []byte {
				t.Helper()

				return withSANsCertPEM(t, &SignOptions{
					IPAddresses: []net.IP{net.ParseIP("192.168.1.10"), net.ParseIP("10.0.0.1")},
				})
			},
			opts: &SignOptions{
				IPAddresses: []net.IP{net.ParseIP("192.168.1.10")},
			},
			want: true,
		},
		{
			name: "missing_ip_does_not_match",
			certPEM: func(t *testing.T) []byte {
				t.Helper()

				return withSANsCertPEM(t, &SignOptions{
					IPAddresses: []net.IP{net.ParseIP("192.168.1.10")},
				})
			},
			opts: &SignOptions{
				IPAddresses: []net.IP{net.ParseIP("203.0.113.5")},
			},
			want: false,
		},
		{
			name: "all_requested_dns_present_matches",
			certPEM: func(t *testing.T) []byte {
				t.Helper()

				return withSANsCertPEM(t, &SignOptions{
					DNSNames: []string{"a.example.com", "b.example.com"},
				})
			},
			opts: &SignOptions{
				DNSNames: []string{"a.example.com"},
			},
			want: true,
		},
		{
			name: "missing_dns_does_not_match",
			certPEM: func(t *testing.T) []byte {
				t.Helper()

				return withSANsCertPEM(t, &SignOptions{
					DNSNames: []string{"a.example.com"},
				})
			},
			opts: &SignOptions{
				DNSNames: []string{"missing.example.com"},
			},
			want: false,
		},
		{
			name: "malformed_pem_does_not_match",
			certPEM: func(_ *testing.T) []byte {
				return []byte("not a pem")
			},
			opts: &SignOptions{
				DNSNames: []string{"any.example.com"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			certPEM := tt.certPEM(t)

			// ACT
			got := certificateMatchesSANs(certPEM, tt.opts)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestApplySignOptions(t *testing.T) {
	emailOID := []int{1, 2, 840, 113549, 1, 9, 1}

	tests := []struct {
		name   string
		opts   *SignOptions
		assert func(t *testing.T, name pkix.Name)
	}{
		{
			name: "nil_opts_is_noop",
			opts: nil,
			assert: func(t *testing.T, name pkix.Name) {
				t.Helper()
				assert.Equal(t, "base", name.CommonName, "nil opts must leave subject untouched")
				assert.Empty(t, name.ExtraNames)
			},
		},
		{
			name: "common_name_set",
			opts: &SignOptions{CommonName: "cn.example.com"},
			assert: func(t *testing.T, name pkix.Name) {
				t.Helper()
				assert.Equal(t, "cn.example.com", name.CommonName)
			},
		},
		{
			name: "empty_common_name_keeps_existing",
			opts: &SignOptions{CommonName: ""},
			assert: func(t *testing.T, name pkix.Name) {
				t.Helper()
				assert.Equal(t, "base", name.CommonName, "empty CommonName must not overwrite")
			},
		},
		{
			name: "organization_set",
			opts: &SignOptions{Organization: "Org Inc"},
			assert: func(t *testing.T, name pkix.Name) {
				t.Helper()
				require.Len(t, name.Organization, 1)
				assert.Equal(t, "Org Inc", name.Organization[0])
			},
		},
		{
			name: "country_set",
			opts: &SignOptions{Country: "FR"},
			assert: func(t *testing.T, name pkix.Name) {
				t.Helper()
				require.Len(t, name.Country, 1)
				assert.Equal(t, "FR", name.Country[0])
			},
		},
		{
			name: "state_maps_to_province",
			opts: &SignOptions{State: "Bavaria"},
			assert: func(t *testing.T, name pkix.Name) {
				t.Helper()
				require.Len(t, name.Province, 1)
				assert.Equal(t, "Bavaria", name.Province[0])
			},
		},
		{
			name: "locality_set",
			opts: &SignOptions{Locality: "Munich"},
			assert: func(t *testing.T, name pkix.Name) {
				t.Helper()
				require.Len(t, name.Locality, 1)
				assert.Equal(t, "Munich", name.Locality[0])
			},
		},
		{
			name: "organizational_unit_set",
			opts: &SignOptions{OrganizationalUnit: "Engineering"},
			assert: func(t *testing.T, name pkix.Name) {
				t.Helper()
				require.Len(t, name.OrganizationalUnit, 1)
				assert.Equal(t, "Engineering", name.OrganizationalUnit[0])
			},
		},
		{
			name: "email_adds_single_extra_name",
			opts: &SignOptions{Email: "person@example.com"},
			assert: func(t *testing.T, name pkix.Name) {
				t.Helper()
				require.Len(t, name.ExtraNames, 1)
				assert.True(t, name.ExtraNames[0].Type.Equal(emailOID), "email OID mismatch")
				assert.Equal(t, "person@example.com", name.ExtraNames[0].Value)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			subject := pkix.Name{CommonName: "base"}

			// ACT
			applySignOptions(&subject, tt.opts)

			// ASSERT
			tt.assert(t, subject)
		})
	}
}

func validateSignOptions(t *testing.T, cert *x509.Certificate, opts *SignOptions) {
	t.Helper()

	if opts.CommonName != "" {
		assert.Equal(t, opts.CommonName, cert.Subject.CommonName)
	}
	if opts.Organization != "" {
		require.Len(t, cert.Subject.Organization, 1)
		assert.Equal(t, opts.Organization, cert.Subject.Organization[0])
	}
	if opts.Country != "" {
		require.Len(t, cert.Subject.Country, 1)
		assert.Equal(t, opts.Country, cert.Subject.Country[0])
	}
	if opts.State != "" {
		require.Len(t, cert.Subject.Province, 1)
		assert.Equal(t, opts.State, cert.Subject.Province[0])
	}
	if opts.Locality != "" {
		require.Len(t, cert.Subject.Locality, 1)
		assert.Equal(t, opts.Locality, cert.Subject.Locality[0])
	}
	if opts.OrganizationalUnit != "" {
		require.Len(t, cert.Subject.OrganizationalUnit, 1)
		assert.Equal(t, opts.OrganizationalUnit, cert.Subject.OrganizationalUnit[0])
	}
	if opts.Email != "" {
		assertSubjectEmail(t, cert, opts.Email)
	}
}

// assertSubjectEmail checks that the emailAddress attribute (OID 1.2.840.113549.1.9.1)
// is present in the certificate subject. applySignOptions stores the email in the
// subject DN, so after parsing it surfaces in Subject.Names rather than in the
// EmailAddresses SAN list.
func assertSubjectEmail(t *testing.T, cert *x509.Certificate, email string) {
	t.Helper()

	emailOID := []int{1, 2, 840, 113549, 1, 9, 1}
	found := slices.ContainsFunc(cert.Subject.Names, func(atv pkix.AttributeTypeAndValue) bool {
		return atv.Type.Equal(emailOID) && atv.Value == email
	})
	assert.True(t, found, "email %s must be present in subject DN", email)
}

func validateSANs(t *testing.T, cert *x509.Certificate, opts *SignOptions) {
	t.Helper()

	for _, ip := range opts.IPAddresses {
		assert.True(t,
			slices.ContainsFunc(cert.IPAddresses, ip.Equal),
			"certificate must contain IP SAN %s", ip)
	}
	for _, dns := range opts.DNSNames {
		assert.Contains(t, cert.DNSNames, dns, "certificate must contain DNS SAN")
	}
}

func generateTestCSR(t *testing.T) string {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "test.example.com",
			Organization: []string{"Test"},
		},
		SignatureAlgorithm: x509.SHA256WithRSA,
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, privateKey)
	require.NoError(t, err)

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	return string(csrPEM)
}

// tamperCSRSignature flips bytes near the end of a valid CSR's DER so the
// structure still parses but the embedded signature no longer verifies.
func tamperCSRSignature(t *testing.T, csrPEM string) string {
	t.Helper()

	block, _ := pem.Decode([]byte(csrPEM))
	require.NotNil(t, block)

	der := slices.Clone(block.Bytes)
	require.Greater(t, len(der), 8)
	for i := len(der) - 8; i < len(der); i++ {
		der[i] ^= 0xFF
	}

	tampered := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: der,
	})

	return string(tampered)
}

// seedRootMaterial returns an in-memory FileManager pre-populated with a real,
// parseable root certificate and key.
func seedRootMaterial(t *testing.T) *files.InMemoryFileManager {
	t.Helper()

	inner := files.NewInMemoryFileManager()
	bootstrap := NewService(inner)
	ctx := context.Background()

	_, err := bootstrap.Root(ctx)
	require.NoError(t, err)
	_, err = bootstrap.RootKey(ctx)
	require.NoError(t, err)

	return inner
}

// seedLeafPair writes a real leaf certificate/key pair (signed by a freshly
// generated root) into inner at the EnsureGenerated paths.
func seedLeafPair(t *testing.T, inner *files.InMemoryFileManager, opts *SignOptions) {
	t.Helper()

	bootstrap := NewService(inner)
	_, _, err := bootstrap.Generate(context.Background(), ensureCertPath, ensureKeyPath, opts)
	require.NoError(t, err)
}

// recordingLeafWrites wraps inner so that writes to the leaf certificate or key
// paths are recorded, while root CA writes and all reads delegate to inner.
func recordingLeafWrites(inner *files.InMemoryFileManager, leafWrites *[]string) files.FileManager {
	return &files.MockFileManager{
		ExistsFunc: inner.Exists,
		ReadFunc:   inner.Read,
		WriteFunc: func(ctx context.Context, path string, data []byte) error {
			if path == ensureCertPath || path == ensureKeyPath {
				*leafWrites = append(*leafWrites, path)
			}

			return inner.Write(ctx, path, data)
		},
	}
}
