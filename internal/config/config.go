package config

import (
	"crypto/tls"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/gameap/gameap/internal/certificates"
	"github.com/pkg/errors"
)

type Config struct {
	HTTPHost   string `env:"HTTP_HOST" envDefault:"0.0.0.0"`
	HTTPBindIP string `env:"HTTP_BIND_IP" envDefault:""`
	HTTPPort   uint16 `env:"HTTP_PORT" envDefault:"8025"`
	HTTPSPort  uint16 `env:"HTTPS_PORT" envDefault:"443"`

	// HTTPAllowedOrigins, when non-empty, becomes the CORS allow-list
	// verbatim (must be fully-qualified origins like "https://app.example.com").
	// When empty, the middleware auto-derives a single origin from HTTPHost,
	// HTTPPort and TLS.ForceHTTPS.
	HTTPAllowedOrigins []string `env:"HTTP_ALLOWED_ORIGINS" envDefault:"" envSeparator:","`

	TLS struct {
		CertFile   string `env:"TLS_CERT_FILE" envDefault:""`
		KeyFile    string `env:"TLS_KEY_FILE" envDefault:""`
		Cert       string `env:"TLS_CERT" envDefault:""`
		Key        string `env:"TLS_KEY" envDefault:""`
		ForceHTTPS bool   `env:"TLS_FORCE_HTTPS" envDefault:"false"`
	}

	ACME struct {
		Enabled              bool          `env:"ACME_ENABLED" envDefault:"false"`
		ChallengeType        string        `env:"ACME_CHALLENGE_TYPE" envDefault:"http-01"`
		Email                string        `env:"ACME_EMAIL" envDefault:""`
		Domains              []string      `env:"ACME_DOMAINS" envDefault:"" envSeparator:","`
		DirectoryURL         string        `env:"ACME_DIRECTORY_URL" envDefault:""`
		DNSProvider          string        `env:"ACME_DNS_PROVIDER" envDefault:""`
		RenewalThreshold     time.Duration `env:"ACME_RENEWAL_THRESHOLD" envDefault:"720h"`
		RenewalCheckInterval time.Duration `env:"ACME_RENEWAL_CHECK_INTERVAL" envDefault:"12h"`
		PropagationTimeout   time.Duration `env:"ACME_PROPAGATION_TIMEOUT" envDefault:"180s"`
		StoragePath          string        `env:"ACME_STORAGE_PATH" envDefault:"acme"`
	}

	DatabaseDriver string `env:"DATABASE_DRIVER,required" envDefault:"mysql"`
	DatabaseURL    string `env:"DATABASE_URL,required,notEmpty"`

	EncryptionKey string `env:"ENCRYPTION_KEY" envDefault:""`
	AuthSecret    string `env:"AUTH_SECRET,required,notEmpty" envDefault:""`
	AuthService   string `env:"AUTH_SERVICE" envDefault:"paseto"`

	// ShortLivedTokenTTL bounds the lifetime of single-use authorization
	// tokens issued for contexts where the token must travel in the URL
	// (WebSocket upgrades, file downloads). It is hard-capped at 10s by the
	// issuing handler regardless of a larger configured value.
	ShortLivedTokenTTL time.Duration `env:"AUTH_SHORT_LIVED_TOKEN_TTL" envDefault:"10s"`

	// Auth groups password-policy and related authentication knobs. The
	// older top-level AuthSecret / AuthService / ShortLivedTokenTTL fields
	// predate this nested section and are kept at the top level for
	// operator-env-file backwards compatibility.
	Auth struct {
		// AllowWeakPasswords disables the common-password blocklist check
		// in auth.ValidatePassword (OWASP ASVS 4.0.3 §2.1.7). When true,
		// the application logs a startup slog.Warn. Length checks
		// (§2.1.1 / §2.1.2) still apply regardless.
		AllowWeakPasswords bool `env:"AUTH_ALLOW_WEAK_PASSWORDS" envDefault:"false"`
	}

	RBAC struct {
		CacheTTL string `env:"RBAC_CACHE_TTL" envDefault:"30s"`
	}

	Cache struct {
		Driver string `env:"CACHE_DRIVER" envDefault:"memory"`

		Redis struct {
			Addr     string `env:"CACHE_REDIS_ADDR" envDefault:"localhost:6379"`
			Password string `env:"CACHE_REDIS_PASSWORD" envDefault:""`
			DB       int    `env:"CACHE_REDIS_DB" envDefault:"0"`
		}

		// TTL configurations for different cache types
		TTL struct {
			RBAC           string `env:"CACHE_TTL_RBAC" envDefault:"24h"`
			Games          string `env:"CACHE_TTL_GAMES" envDefault:"48h"`
			Nodes          string `env:"CACHE_TTL_NODES" envDefault:"24h"`
			Users          string `env:"CACHE_TTL_USERS" envDefault:"6h"`
			PersonalTokens string `env:"CACHE_TTL_PERSONAL_TOKENS" envDefault:"24h"`
			ServerSettings string `env:"CACHE_TTL_SERVER_SETTINGS" envDefault:"12h"`
		}
	}

	Files struct {
		Driver string `env:"FILES_DRIVER" envDefault:"local"`

		Local struct {
			BasePath string `env:"FILES_LOCAL_BASE_PATH" envDefault:""`
		}

		S3 struct {
			Endpoint        string `env:"FILES_S3_ENDPOINT" envDefault:""`
			UseSSL          bool   `env:"FILES_S3_USE_SSL" envDefault:"true"`
			AccessKeyID     string `env:"FILES_S3_ACCESS_KEY_ID" envDefault:""`
			SecretAccessKey string `env:"FILES_S3_SECRET_ACCESS_KEY" envDefault:""`
			Bucket          string `env:"FILES_S3_BUCKET" envDefault:""`
		}

		Upload struct {
			ChunkSize       ByteSize      `env:"FILES_UPLOAD_CHUNK_SIZE" envDefault:"8M"`
			SessionTTL      time.Duration `env:"FILES_UPLOAD_SESSION_TTL" envDefault:"24h"`
			MaxChunks       uint          `env:"FILES_UPLOAD_MAX_CHUNKS" envDefault:"100000"`
			DispatchTimeout time.Duration `env:"FILES_UPLOAD_DISPATCH_TIMEOUT" envDefault:"2m"`
			JanitorInterval time.Duration `env:"FILES_UPLOAD_JANITOR_INTERVAL" envDefault:"12h"`
		}

		Archive struct {
			MaxBytes            ByteSize `env:"FILES_ARCHIVE_MAX_BYTES" envDefault:"100G"`
			MaxFiles            uint32   `env:"FILES_ARCHIVE_MAX_FILES" envDefault:"500000"`
			ConcurrentPerServer uint32   `env:"FILES_ARCHIVE_CONCURRENT_PER_SERVER" envDefault:"2"`
		}
	}

	Logger struct {
		Level        string `env:"LOGGER_LEVEL" envDefault:"info"`
		LogDBQueries bool   `env:"LOGGER_LOG_DB_QUERIES" envDefault:"false"`
	}

	Audit struct {
		// Enabled toggles the structured security audit log (auth
		// failures, access-control denials, sensitive operations).
		Enabled bool `env:"AUDIT_ENABLED" envDefault:"true"`
		// ClientIPHeader is the trusted reverse-proxy header to read the
		// real client IP from (e.g. X-Real-IP); empty uses RemoteAddr.
		ClientIPHeader string `env:"AUDIT_CLIENT_IP_HEADER" envDefault:""`
	}

	GlobalAPI struct {
		URL string `env:"GLOBAL_API_URL" envDefault:"https://api.gameap.com"`
	}

	UI struct {
		DefaultLanguage string `env:"DEFAULT_LANGUAGE" envDefault:""`
	}

	Plugins struct {
		Disabled bool     `env:"PLUGINS_DISABLED" envDefault:"false"`
		AutoLoad []string `env:"PLUGINS_AUTOLOAD" envDefault:"" envSeparator:","`
		Cache    struct {
			Enabled bool   `env:"PLUGINS_CACHE_ENABLED" envDefault:"true"`
			Dir     string `env:"PLUGINS_CACHE_DIR" envDefault:""`
		}
	}

	PluginStore struct {
		URL        string `env:"PLUGIN_STORE_URL" envDefault:"https://plugins.gameap.dev/api"`
		LicenseKey string `env:"PLUGIN_STORE_LICENSE_KEY" envDefault:""`
	}

	// Captcha protects the login endpoint against automated credential
	// stuffing. Empty Provider disables it; SecretKey stays server-side and
	// is never exposed through /api/config/public.
	Captcha struct {
		// Provider: "" (disabled), "recaptcha_v2", "recaptcha_v3" or "turnstile".
		Provider  string `env:"CAPTCHA_PROVIDER" envDefault:""`
		SiteKey   string `env:"CAPTCHA_SITE_KEY" envDefault:""`
		SecretKey string `env:"CAPTCHA_SECRET_KEY" envDefault:""`
		// MinScore is the reCAPTCHA v3 pass threshold (0.0–1.0); ignored by
		// the checkbox/Turnstile providers that only return success.
		MinScore float64 `env:"CAPTCHA_MIN_SCORE" envDefault:"0.5"`
		// FailOpen lets a login through when the upstream siteverify call
		// itself fails (network/5xx). Defaults to fail-closed.
		FailOpen bool `env:"CAPTCHA_FAIL_OPEN" envDefault:"false"`
		// VerifyURL overrides the provider's siteverify endpoint (egress
		// proxies, tests). Empty uses the provider default.
		VerifyURL string `env:"CAPTCHA_VERIFY_URL" envDefault:""`
	}

	// Security configures the global HTTP response security headers emitted by
	// SecurityHeadersMiddleware (HSTS, X-Content-Type-Options, X-Frame-Options,
	// Referrer-Policy and Content-Security-Policy). Secure defaults; every
	// directive is env-overridable so self-hosted deployments can adapt the
	// Content-Security-Policy for plugins or reverse-proxy setups without
	// patching the binary.
	Security struct {
		Enabled bool `env:"SECURITY_HEADERS_ENABLED" envDefault:"true"`

		HSTS struct {
			// Enabled is the HSTS toggle. HSTS is only emitted on HTTPS
			// requests (r.TLS != nil, X-Forwarded-Proto: https, or
			// TLS.ForceHTTPS) regardless of this flag.
			Enabled           bool `env:"SECURITY_HSTS_ENABLED" envDefault:"true"`
			MaxAge            int  `env:"SECURITY_HSTS_MAX_AGE" envDefault:"31536000"`
			IncludeSubDomains bool `env:"SECURITY_HSTS_INCLUDE_SUBDOMAINS" envDefault:"false"`
			Preload           bool `env:"SECURITY_HSTS_PRELOAD" envDefault:"false"`
		}

		ContentTypeOptions bool   `env:"SECURITY_CONTENT_TYPE_OPTIONS" envDefault:"true"`
		FrameOptions       string `env:"SECURITY_FRAME_OPTIONS" envDefault:"SAMEORIGIN"`
		ReferrerPolicy     string `env:"SECURITY_REFERRER_POLICY" envDefault:"strict-origin-when-cross-origin"`

		CSP struct {
			Enabled    bool `env:"SECURITY_CSP_ENABLED" envDefault:"true"`
			ReportOnly bool `env:"SECURITY_CSP_REPORT_ONLY" envDefault:"false"`
			// Policy, when non-empty, replaces the generated CSP verbatim
			// (captcha-aware logic and extra-src lists are then ignored).
			Policy    string `env:"SECURITY_CSP_POLICY" envDefault:""`
			ReportURI string `env:"SECURITY_CSP_REPORT_URI" envDefault:""`

			// Additive allow-lists appended to the generated directives so
			// plugins / reverse proxies can extend the policy without
			// replacing it wholesale.
			ExtraScriptSrc  []string `env:"SECURITY_CSP_EXTRA_SCRIPT_SRC" envSeparator:"," envDefault:""`
			ExtraStyleSrc   []string `env:"SECURITY_CSP_EXTRA_STYLE_SRC" envSeparator:"," envDefault:""`
			ExtraConnectSrc []string `env:"SECURITY_CSP_EXTRA_CONNECT_SRC" envSeparator:"," envDefault:""`
			ExtraImgSrc     []string `env:"SECURITY_CSP_EXTRA_IMG_SRC" envSeparator:"," envDefault:""`
			ExtraFrameSrc   []string `env:"SECURITY_CSP_EXTRA_FRAME_SRC" envSeparator:"," envDefault:""`
			ExtraFontSrc    []string `env:"SECURITY_CSP_EXTRA_FONT_SRC" envSeparator:"," envDefault:""`
		}
	}

	PubSub struct {
		Driver     string `env:"PUBSUB_DRIVER" envDefault:"memory"`
		InstanceID string `env:"PUBSUB_INSTANCE_ID" envDefault:""`

		Retry struct {
			Enabled      bool    `env:"PUBSUB_RETRY_ENABLED" envDefault:"true"`
			MaxRetries   int     `env:"PUBSUB_RETRY_MAX_RETRIES" envDefault:"3"`
			InitialDelay string  `env:"PUBSUB_RETRY_INITIAL_DELAY" envDefault:"100ms"`
			MaxDelay     string  `env:"PUBSUB_RETRY_MAX_DELAY" envDefault:"5s"`
			Multiplier   float64 `env:"PUBSUB_RETRY_MULTIPLIER" envDefault:"2.0"`
		}

		DLQ struct {
			Enabled bool   `env:"PUBSUB_DLQ_ENABLED" envDefault:"false"`
			Driver  string `env:"PUBSUB_DLQ_DRIVER" envDefault:"memory"`
			MaxSize int    `env:"PUBSUB_DLQ_MAX_SIZE" envDefault:"1000"`
		}

		Redis struct {
			Addr     string `env:"PUBSUB_REDIS_ADDR" envDefault:""`
			Password string `env:"PUBSUB_REDIS_PASSWORD" envDefault:""`
			DB       int    `env:"PUBSUB_REDIS_DB" envDefault:"1"`
		}
	}

	GRPC struct {
		TLSEnabled           bool   `env:"GRPC_TLS_ENABLED" envDefault:"true"`
		Port                 uint16 `env:"GRPC_PORT" envDefault:"31718"`
		MaxRecvMsgSize       int    `env:"GRPC_MAX_RECV_MSG_SIZE" envDefault:"10485760"`
		MaxSendMsgSize       int    `env:"GRPC_MAX_SEND_MSG_SIZE" envDefault:"10485760"`
		MaxConcurrentStreams uint32 `env:"GRPC_MAX_CONCURRENT_STREAMS" envDefault:"100"`
		FileTransferBasePath string `env:"GRPC_FILE_TRANSFER_BASE_PATH" envDefault:""`
		RequireMTLS          bool   `env:"GRPC_REQUIRE_MTLS" envDefault:"false"`
		EnableReflection     bool   `env:"GRPC_ENABLE_REFLECTION" envDefault:"false"`
		ExternalHost         string `env:"GRPC_EXTERNAL_HOST" envDefault:""`
		ExternalPort         uint16 `env:"GRPC_EXTERNAL_PORT" envDefault:"0"`
	}

	DaemonSetupKey string `env:"DAEMON_SETUP_KEY" envDefault:""`

	TaskReaper struct {
		Interval       time.Duration `env:"TASK_REAPER_INTERVAL" envDefault:"1m"`
		StaleThreshold time.Duration `env:"TASK_REAPER_STALE_THRESHOLD" envDefault:"10m"`
	}
}

func LoadConfig() (*Config, error) {
	var cfg Config
	var err error

	if cfg, err = env.ParseAs[Config](); err != nil {
		return nil, errors.WithMessage(err, "failed to parse config")
	}

	setDefaultConfigValues(&cfg)

	normalizeConfigValues(&cfg)

	return &cfg, nil
}

// LetsEncryptProductionDirectoryURL is the production ACME directory used
// when ACME_DIRECTORY_URL is not explicitly set.
const LetsEncryptProductionDirectoryURL = "https://acme-v02.api.letsencrypt.org/directory"

func setDefaultConfigValues(cfg *Config) {
	if cfg.ACME.DirectoryURL == "" {
		cfg.ACME.DirectoryURL = LetsEncryptProductionDirectoryURL
	}
}

func normalizeConfigValues(cfg *Config) {
	cfg.DatabaseDriver = strings.ToLower(cfg.DatabaseDriver)

	switch cfg.DatabaseDriver {
	case "postgres", "postgresql", "pgx", "pg", "pgsql": //nolint:goconst
		cfg.DatabaseDriver = "pgx"
	}

	cfg.Cache.Driver = strings.ToLower(cfg.Cache.Driver)
	switch cfg.Cache.Driver {
	case "postgres", "postgresql", "pgx", "pg", "pgsql": //nolint:goconst,nolintlint
		cfg.Cache.Driver = "postgres"
	}

	cfg.UI.DefaultLanguage = strings.ToLower(cfg.UI.DefaultLanguage)

	cfg.PubSub.Driver = strings.ToLower(cfg.PubSub.Driver)
	switch cfg.PubSub.Driver {
	case "postgres", "postgresql", "pgx", "pg", "pgsql": //nolint:goconst,nolintlint
		cfg.PubSub.Driver = "postgres"
	case "inmemory":
		cfg.PubSub.Driver = "memory"
	}
}

func (c *Config) TLSEnabled() bool {
	return c.EffectiveCertSource() != CertSourceNone
}

const (
	ACMEChallengeHTTP01 = "http-01"
	ACMEChallengeDNS01  = "dns-01"
)

func (c *Config) ACMEEnabled() bool {
	if !c.ACME.Enabled || c.ACME.Email == "" || len(c.ACME.Domains) == 0 {
		return false
	}

	switch c.ACME.ChallengeType {
	case ACMEChallengeHTTP01:
		return true
	case ACMEChallengeDNS01:
		return c.ACME.DNSProvider != ""
	default:
		return false
	}
}

type CertSource int

const (
	CertSourceNone CertSource = iota
	CertSourceACME
	CertSourceFile
	CertSourceInline
)

func (s CertSource) String() string {
	switch s {
	case CertSourceACME:
		return "acme"
	case CertSourceFile:
		return "file"
	case CertSourceInline:
		return "inline"
	default:
		return "none"
	}
}

func (c *Config) EffectiveCertSource() CertSource {
	if c.ACMEEnabled() {
		return CertSourceACME
	}

	if c.TLS.CertFile != "" && c.TLS.KeyFile != "" {
		return CertSourceFile
	}

	if c.TLS.Cert != "" && c.TLS.Key != "" {
		return CertSourceInline
	}

	return CertSourceNone
}

func (c *Config) LoadTLSCertificate() (*tls.Certificate, error) {
	if c.TLS.CertFile != "" && c.TLS.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.TLS.CertFile, c.TLS.KeyFile)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load TLS certificate from files")
		}

		return &cert, nil
	}

	certPEM := certificates.DecodePossibleBase64(c.TLS.Cert)
	keyPEM := certificates.DecodePossibleBase64(c.TLS.Key)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load TLS certificate from content")
	}

	return &cert, nil
}
