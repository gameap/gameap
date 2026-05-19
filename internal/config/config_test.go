package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	t.Run("with_required_env_vars", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "mysql://localhost/test")
		t.Setenv("AUTH_SECRET", "test-secret")

		cfg, err := LoadConfig()
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Equal(t, "mysql://localhost/test", cfg.DatabaseURL)
		assert.Equal(t, "test-secret", cfg.AuthSecret)
	})

	t.Run("without_database_url", func(t *testing.T) {
		t.Setenv("AUTH_SECRET", "test-secret")

		_, err := LoadConfig()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DATABASE_URL")
	})

	t.Run("without_auth_secret", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "mysql://localhost/test")

		_, err := LoadConfig()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AUTH_SECRET")
	})

	t.Run("default_values", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "mysql://localhost/test")
		t.Setenv("AUTH_SECRET", "test-secret")

		cfg, err := LoadConfig()
		require.NoError(t, err)

		assert.Equal(t, "0.0.0.0", cfg.HTTPHost)
		assert.Equal(t, uint16(8025), cfg.HTTPPort)
		assert.Equal(t, uint16(443), cfg.HTTPSPort)
		assert.Equal(t, "mysql", cfg.DatabaseDriver)
		assert.Equal(t, "paseto", cfg.AuthService)
		assert.Equal(t, "30s", cfg.RBAC.CacheTTL)
		assert.Equal(t, "memory", cfg.Cache.Driver)
		assert.Equal(t, "local", cfg.Files.Driver)
		assert.Equal(t, "info", cfg.Logger.Level)
		assert.False(t, cfg.Logger.LogDBQueries)
		assert.Equal(t, "https://api.gameap.com", cfg.GlobalAPI.URL)
	})

	t.Run("default_archive_values", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "mysql://localhost/test")
		t.Setenv("AUTH_SECRET", "test-secret")

		cfg, err := LoadConfig()
		require.NoError(t, err)

		assert.Equal(t, uint64(100*1024*1024*1024), cfg.Files.Archive.MaxBytes.Uint64(),
			"FILES_ARCHIVE_MAX_BYTES default must be 100 GiB")
		assert.Equal(t, uint32(500_000), cfg.Files.Archive.MaxFiles,
			"FILES_ARCHIVE_MAX_FILES default must be 500 000")
		assert.Equal(t, uint32(2), cfg.Files.Archive.ConcurrentPerServer,
			"FILES_ARCHIVE_CONCURRENT_PER_SERVER default must be 2")
	})

	t.Run("archive_env_overrides", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "mysql://localhost/test")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("FILES_ARCHIVE_MAX_BYTES", "10G")
		t.Setenv("FILES_ARCHIVE_MAX_FILES", "100")
		t.Setenv("FILES_ARCHIVE_CONCURRENT_PER_SERVER", "5")

		cfg, err := LoadConfig()
		require.NoError(t, err)

		assert.Equal(t, uint64(10*1024*1024*1024), cfg.Files.Archive.MaxBytes.Uint64())
		assert.Equal(t, uint32(100), cfg.Files.Archive.MaxFiles)
		assert.Equal(t, uint32(5), cfg.Files.Archive.ConcurrentPerServer)
	})
}

func TestNormalizeConfigValues(t *testing.T) {
	tests := []struct {
		name                   string
		databaseDriver         string
		cacheDriver            string
		expectedDatabaseDriver string
		expectedCacheDriver    string
	}{
		{
			name:                   "postgres_normalized",
			databaseDriver:         "postgres",
			cacheDriver:            "memory",
			expectedDatabaseDriver: "pgx",
			expectedCacheDriver:    "memory",
		},
		{
			name:                   "postgresql_normalized",
			databaseDriver:         "postgresql",
			cacheDriver:            "memory",
			expectedDatabaseDriver: "pgx",
			expectedCacheDriver:    "memory",
		},
		{
			name:                   "pgx_unchanged",
			databaseDriver:         "pgx",
			cacheDriver:            "memory",
			expectedDatabaseDriver: "pgx",
			expectedCacheDriver:    "memory",
		},
		{
			name:                   "pg_normalized",
			databaseDriver:         "pg",
			cacheDriver:            "memory",
			expectedDatabaseDriver: "pgx",
			expectedCacheDriver:    "memory",
		},
		{
			name:                   "pgsql_normalized",
			databaseDriver:         "pgsql",
			cacheDriver:            "memory",
			expectedDatabaseDriver: "pgx",
			expectedCacheDriver:    "memory",
		},
		{
			name:                   "mysql_unchanged",
			databaseDriver:         "mysql",
			cacheDriver:            "memory",
			expectedDatabaseDriver: "mysql",
			expectedCacheDriver:    "memory",
		},
		{
			name:                   "uppercase_postgres_normalized",
			databaseDriver:         "POSTGRES",
			cacheDriver:            "memory",
			expectedDatabaseDriver: "pgx",
			expectedCacheDriver:    "memory",
		},
		{
			name:                   "cache_postgres_normalized",
			databaseDriver:         "mysql",
			cacheDriver:            "postgres",
			expectedDatabaseDriver: "mysql",
			expectedCacheDriver:    "postgres",
		},
		{
			name:                   "cache_postgresql_normalized",
			databaseDriver:         "mysql",
			cacheDriver:            "postgresql",
			expectedDatabaseDriver: "mysql",
			expectedCacheDriver:    "postgres",
		},
		{
			name:                   "cache_pgx_normalized",
			databaseDriver:         "mysql",
			cacheDriver:            "pgx",
			expectedDatabaseDriver: "mysql",
			expectedCacheDriver:    "postgres",
		},
		{
			name:                   "cache_pg_normalized",
			databaseDriver:         "mysql",
			cacheDriver:            "pg",
			expectedDatabaseDriver: "mysql",
			expectedCacheDriver:    "postgres",
		},
		{
			name:                   "cache_pgsql_normalized",
			databaseDriver:         "mysql",
			cacheDriver:            "pgsql",
			expectedDatabaseDriver: "mysql",
			expectedCacheDriver:    "postgres",
		},
		{
			name:                   "cache_redis_unchanged",
			databaseDriver:         "mysql",
			cacheDriver:            "redis",
			expectedDatabaseDriver: "mysql",
			expectedCacheDriver:    "redis",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := &Config{
				DatabaseDriver: test.databaseDriver,
			}
			cfg.Cache.Driver = test.cacheDriver

			normalizeConfigValues(cfg)

			assert.Equal(t, test.expectedDatabaseDriver, cfg.DatabaseDriver)
			assert.Equal(t, test.expectedCacheDriver, cfg.Cache.Driver)
		})
	}
}

func TestConfig_TLSEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected bool
	}{
		{
			name:     "no_tls_configured",
			config:   Config{},
			expected: false,
		},
		{
			name: "cert_and_key_files_set",
			config: Config{
				TLS: struct {
					CertFile   string `env:"TLS_CERT_FILE" envDefault:""`
					KeyFile    string `env:"TLS_KEY_FILE" envDefault:""`
					Cert       string `env:"TLS_CERT" envDefault:""`
					Key        string `env:"TLS_KEY" envDefault:""`
					ForceHTTPS bool   `env:"TLS_FORCE_HTTPS" envDefault:"false"`
				}{
					CertFile: "/path/to/cert.pem",
					KeyFile:  "/path/to/key.pem",
				},
			},
			expected: true,
		},
		{
			name: "cert_and_key_content_set",
			config: Config{
				TLS: struct {
					CertFile   string `env:"TLS_CERT_FILE" envDefault:""`
					KeyFile    string `env:"TLS_KEY_FILE" envDefault:""`
					Cert       string `env:"TLS_CERT" envDefault:""`
					Key        string `env:"TLS_KEY" envDefault:""`
					ForceHTTPS bool   `env:"TLS_FORCE_HTTPS" envDefault:"false"`
				}{
					Cert: "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----",
					Key:  "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----",
				},
			},
			expected: true,
		},
		{
			name: "only_cert_file_set",
			config: Config{
				TLS: struct {
					CertFile   string `env:"TLS_CERT_FILE" envDefault:""`
					KeyFile    string `env:"TLS_KEY_FILE" envDefault:""`
					Cert       string `env:"TLS_CERT" envDefault:""`
					Key        string `env:"TLS_KEY" envDefault:""`
					ForceHTTPS bool   `env:"TLS_FORCE_HTTPS" envDefault:"false"`
				}{
					CertFile: "/path/to/cert.pem",
				},
			},
			expected: false,
		},
		{
			name: "only_key_file_set",
			config: Config{
				TLS: struct {
					CertFile   string `env:"TLS_CERT_FILE" envDefault:""`
					KeyFile    string `env:"TLS_KEY_FILE" envDefault:""`
					Cert       string `env:"TLS_CERT" envDefault:""`
					Key        string `env:"TLS_KEY" envDefault:""`
					ForceHTTPS bool   `env:"TLS_FORCE_HTTPS" envDefault:"false"`
				}{
					KeyFile: "/path/to/key.pem",
				},
			},
			expected: false,
		},
		{
			name: "only_cert_content_set",
			config: Config{
				TLS: struct {
					CertFile   string `env:"TLS_CERT_FILE" envDefault:""`
					KeyFile    string `env:"TLS_KEY_FILE" envDefault:""`
					Cert       string `env:"TLS_CERT" envDefault:""`
					Key        string `env:"TLS_KEY" envDefault:""`
					ForceHTTPS bool   `env:"TLS_FORCE_HTTPS" envDefault:"false"`
				}{
					Cert: "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----",
				},
			},
			expected: false,
		},
		{
			name: "only_key_content_set",
			config: Config{
				TLS: struct {
					CertFile   string `env:"TLS_CERT_FILE" envDefault:""`
					KeyFile    string `env:"TLS_KEY_FILE" envDefault:""`
					Cert       string `env:"TLS_CERT" envDefault:""`
					Key        string `env:"TLS_KEY" envDefault:""`
					ForceHTTPS bool   `env:"TLS_FORCE_HTTPS" envDefault:"false"`
				}{
					Key: "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----",
				},
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.config.TLSEnabled()
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestConfig_LoadTLSCertificate(t *testing.T) {
	certPEM, keyPEM := generateTestCertificate(t)

	t.Run("from_files", func(t *testing.T) {
		tempDir := t.TempDir()
		certFile := filepath.Join(tempDir, "cert.pem")
		keyFile := filepath.Join(tempDir, "key.pem")

		err := os.WriteFile(certFile, certPEM, 0o600)
		require.NoError(t, err)
		err = os.WriteFile(keyFile, keyPEM, 0o600)
		require.NoError(t, err)

		cfg := &Config{}
		cfg.TLS.CertFile = certFile
		cfg.TLS.KeyFile = keyFile

		cert, err := cfg.LoadTLSCertificate()
		require.NoError(t, err)
		assert.NotNil(t, cert)
	})

	t.Run("from_content", func(t *testing.T) {
		cfg := &Config{}
		cfg.TLS.Cert = string(certPEM)
		cfg.TLS.Key = string(keyPEM)

		cert, err := cfg.LoadTLSCertificate()
		require.NoError(t, err)
		assert.NotNil(t, cert)
	})

	t.Run("from_base64_content", func(t *testing.T) {
		cfg := &Config{}
		cfg.TLS.Cert = base64.StdEncoding.EncodeToString(certPEM)
		cfg.TLS.Key = base64.StdEncoding.EncodeToString(keyPEM)

		cert, err := cfg.LoadTLSCertificate()
		require.NoError(t, err)
		assert.NotNil(t, cert)
	})

	t.Run("invalid_file_paths", func(t *testing.T) {
		cfg := &Config{}
		cfg.TLS.CertFile = "/nonexistent/cert.pem"
		cfg.TLS.KeyFile = "/nonexistent/key.pem"

		cert, err := cfg.LoadTLSCertificate()
		require.Error(t, err)
		assert.Nil(t, cert)
		assert.Contains(t, err.Error(), "failed to load TLS certificate from files")
	})

	t.Run("invalid_content", func(t *testing.T) {
		cfg := &Config{}
		cfg.TLS.Cert = "invalid-cert-content"
		cfg.TLS.Key = "invalid-key-content"

		cert, err := cfg.LoadTLSCertificate()
		require.Error(t, err)
		assert.Nil(t, cert)
		assert.Contains(t, err.Error(), "failed to load TLS certificate from content")
	})

	t.Run("files_take_priority_over_content", func(t *testing.T) {
		tempDir := t.TempDir()
		certFile := filepath.Join(tempDir, "cert.pem")
		keyFile := filepath.Join(tempDir, "key.pem")

		err := os.WriteFile(certFile, certPEM, 0o600)
		require.NoError(t, err)
		err = os.WriteFile(keyFile, keyPEM, 0o600)
		require.NoError(t, err)

		cfg := &Config{}
		cfg.TLS.CertFile = certFile
		cfg.TLS.KeyFile = keyFile
		cfg.TLS.Cert = "invalid-cert-content"
		cfg.TLS.Key = "invalid-key-content"

		cert, err := cfg.LoadTLSCertificate()
		require.NoError(t, err)
		assert.NotNil(t, cert)
	})
}

func TestNormalizeConfigValues_DefaultLanguage(t *testing.T) {
	tests := []struct {
		name             string
		defaultLanguage  string
		expectedLanguage string
	}{
		{
			name:             "empty_unchanged",
			defaultLanguage:  "",
			expectedLanguage: "",
		},
		{
			name:             "en_unchanged",
			defaultLanguage:  "en",
			expectedLanguage: "en",
		},
		{
			name:             "ru_unchanged",
			defaultLanguage:  "ru",
			expectedLanguage: "ru",
		},
		{
			name:             "uppercase_EN_normalized_to_lowercase",
			defaultLanguage:  "EN",
			expectedLanguage: "en",
		},
		{
			name:             "uppercase_RU_normalized_to_lowercase",
			defaultLanguage:  "RU",
			expectedLanguage: "ru",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			cfg.UI.DefaultLanguage = tt.defaultLanguage

			normalizeConfigValues(cfg)

			assert.Equal(t, tt.expectedLanguage, cfg.UI.DefaultLanguage)
		})
	}
}

// =============================================================================
// ACME helpers (Tier 3 G)
// =============================================================================

const (
	testACMEEmail   = "ops@example.com"
	testCertPath    = "/some/cert.pem"
	testCertKeyPath = "/some/key.pem"
)

func TestConfig_ACMEEnabled(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*Config)
		wantResult bool
	}{
		{
			name: "true_when_http01_with_email_and_domains",
			mutate: func(c *Config) {
				c.ACME.Enabled = true
				c.ACME.ChallengeType = ACMEChallengeHTTP01
				c.ACME.Email = testACMEEmail
				c.ACME.Domains = []string{"example.com"}
			},
			wantResult: true,
		},
		{
			name: "true_when_dns01_with_provider",
			mutate: func(c *Config) {
				c.ACME.Enabled = true
				c.ACME.ChallengeType = ACMEChallengeDNS01
				c.ACME.Email = testACMEEmail
				c.ACME.Domains = []string{"example.com"}
				c.ACME.DNSProvider = "cloudflare"
			},
			wantResult: true,
		},
		{
			name: "false_when_acme_disabled",
			mutate: func(c *Config) {
				c.ACME.Enabled = false
				c.ACME.ChallengeType = ACMEChallengeHTTP01
				c.ACME.Email = testACMEEmail
				c.ACME.Domains = []string{"example.com"}
			},
			wantResult: false,
		},
		{
			name: "false_when_email_missing",
			mutate: func(c *Config) {
				c.ACME.Enabled = true
				c.ACME.ChallengeType = ACMEChallengeHTTP01
				c.ACME.Email = ""
				c.ACME.Domains = []string{"example.com"}
			},
			wantResult: false,
		},
		{
			name: "false_when_domains_empty",
			mutate: func(c *Config) {
				c.ACME.Enabled = true
				c.ACME.ChallengeType = ACMEChallengeHTTP01
				c.ACME.Email = testACMEEmail
				c.ACME.Domains = nil
			},
			wantResult: false,
		},
		{
			name: "false_when_dns01_without_provider",
			mutate: func(c *Config) {
				c.ACME.Enabled = true
				c.ACME.ChallengeType = ACMEChallengeDNS01
				c.ACME.Email = testACMEEmail
				c.ACME.Domains = []string{"example.com"}
				c.ACME.DNSProvider = ""
			},
			wantResult: false,
		},
		{
			name: "false_for_unknown_challenge_type",
			mutate: func(c *Config) {
				c.ACME.Enabled = true
				c.ACME.ChallengeType = "tls-alpn-01"
				c.ACME.Email = testACMEEmail
				c.ACME.Domains = []string{"example.com"}
			},
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			cfg := &Config{}
			tt.mutate(cfg)

			// ACT
			got := cfg.ACMEEnabled()

			// ASSERT
			assert.Equal(t, tt.wantResult, got)
		})
	}
}

func TestConfig_EffectiveCertSource(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   CertSource
	}{
		{
			name: "acme_takes_priority_when_enabled",
			mutate: func(c *Config) {
				c.ACME.Enabled = true
				c.ACME.ChallengeType = ACMEChallengeHTTP01
				c.ACME.Email = testACMEEmail
				c.ACME.Domains = []string{"example.com"}
				c.TLS.CertFile = testCertPath
				c.TLS.KeyFile = testCertKeyPath
			},
			want: CertSourceACME,
		},
		{
			name: "file_when_paths_set_and_acme_disabled",
			mutate: func(c *Config) {
				c.ACME.Enabled = false
				c.TLS.CertFile = testCertPath
				c.TLS.KeyFile = testCertKeyPath
			},
			want: CertSourceFile,
		},
		{
			name: "inline_when_only_content_set",
			mutate: func(c *Config) {
				c.TLS.Cert = "-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----"
				c.TLS.Key = "-----BEGIN PRIVATE KEY-----\n-----END PRIVATE KEY-----"
			},
			want: CertSourceInline,
		},
		{
			name:   "none_when_nothing_configured",
			mutate: func(_ *Config) {},
			want:   CertSourceNone,
		},
		{
			name: "none_when_only_cert_path_set",
			mutate: func(c *Config) {
				c.TLS.CertFile = testCertPath
			},
			want: CertSourceNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			cfg := &Config{}
			tt.mutate(cfg)

			// ACT
			got := cfg.EffectiveCertSource()

			// ASSERT
			assert.Equal(t, tt.want, got, "EffectiveCertSource mismatch")
		})
	}
}

func TestCertSource_String(t *testing.T) {
	tests := []struct {
		source CertSource
		want   string
	}{
		{source: CertSourceNone, want: "none"},
		{source: CertSourceACME, want: "acme"},
		{source: CertSourceFile, want: "file"},
		{source: CertSourceInline, want: "inline"},
		{source: CertSource(99), want: "none"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.source.String())
		})
	}
}

func TestConfig_TLSEnabled_FollowsEffectiveCertSource(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   bool
	}{
		{
			name: "true_when_acme_enabled",
			mutate: func(c *Config) {
				c.ACME.Enabled = true
				c.ACME.ChallengeType = ACMEChallengeHTTP01
				c.ACME.Email = testACMEEmail
				c.ACME.Domains = []string{"example.com"}
			},
			want: true,
		},
		{
			name:   "false_when_no_source_configured",
			mutate: func(_ *Config) {},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			tt.mutate(cfg)

			assert.Equal(t, tt.want, cfg.TLSEnabled())
		})
	}
}

func generateTestCertificate(t *testing.T) (certPEM []byte, keyPEM []byte) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	return certPEM, keyPEM
}
