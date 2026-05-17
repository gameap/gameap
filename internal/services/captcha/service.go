package captcha

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

// Provider identifies the captcha vendor. The empty value disables captcha.
type Provider string

const (
	ProviderNone        Provider = ""
	ProviderReCAPTCHAV2 Provider = "recaptcha_v2"
	ProviderReCAPTCHAV3 Provider = "recaptcha_v3"
	ProviderTurnstile   Provider = "turnstile"
)

const (
	recaptchaVerifyURL = "https://www.google.com/recaptcha/api/siteverify"
	turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

	defaultTimeout = 30 * time.Second
)

var (
	// ErrTokenRequired is returned when the client omitted the captcha token.
	ErrTokenRequired = api.NewValidationError("captcha token is required")

	// ErrVerificationFailed is returned when the provider rejected the token
	// (or, for reCAPTCHA v3, the score was below the configured threshold).
	ErrVerificationFailed = api.NewValidationError("captcha verification failed")
)

// Config is the captcha subset of the application config, decoupled from the
// anonymous struct in internal/config so this package stays import-light and
// independently testable.
type Config struct {
	Provider  Provider
	SiteKey   string
	SecretKey string
	MinScore  float64
	FailOpen  bool
	VerifyURL string
}

// Option customises the service at construction time (test seams).
type Option func(*Service)

// WithHTTPClient overrides the HTTP client (tests, custom transport).
func WithHTTPClient(c *http.Client) Option {
	return func(s *Service) {
		if c != nil {
			s.httpClient = c
		}
	}
}

// WithVerifyURL overrides the upstream siteverify endpoint (tests, proxies).
func WithVerifyURL(u string) Option {
	return func(s *Service) {
		if u != "" {
			s.verifyURL = u
		}
	}
}

// Service verifies captcha tokens against the configured provider.
type Service struct {
	provider   Provider
	secretKey  string
	minScore   float64
	failOpen   bool
	verifyURL  string
	httpClient *http.Client
}

// NewService builds a captcha service from cfg. Functional options win over
// cfg (tests point WithVerifyURL at an httptest server).
func NewService(cfg Config, opts ...Option) *Service {
	s := &Service{
		provider:  cfg.Provider,
		secretKey: cfg.SecretKey,
		minScore:  cfg.MinScore,
		failOpen:  cfg.FailOpen,
		verifyURL: resolveVerifyURL(cfg.Provider, cfg.VerifyURL),
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func resolveVerifyURL(provider Provider, override string) string {
	if override != "" {
		return override
	}

	switch provider {
	case ProviderTurnstile:
		return turnstileVerifyURL
	case ProviderReCAPTCHAV2, ProviderReCAPTCHAV3:
		return recaptchaVerifyURL
	case ProviderNone:
		return ""
	default:
		return ""
	}
}

// Enabled reports whether a usable provider is configured.
func (s *Service) Enabled() bool {
	return s.provider != ProviderNone && s.secretKey != "" && s.verifyURL != ""
}

// verifyResponse is the common shape of the reCAPTCHA and Turnstile
// siteverify responses. Score/Action are populated for reCAPTCHA v3 only.
type verifyResponse struct {
	Success    bool     `json:"success"`
	Score      float64  `json:"score"`
	Action     string   `json:"action"`
	Hostname   string   `json:"hostname"`
	ErrorCodes []string `json:"error-codes"`
}

// Verify checks token with the provider. A nil result means the visitor
// solved the captcha. A missing/rejected token returns a 422 validation
// error. An upstream failure returns nil when FailOpen, otherwise a
// 503-mapped error so the login is blocked rather than waved through.
func (s *Service) Verify(ctx context.Context, token, remoteIP string) error {
	if !s.Enabled() {
		return nil
	}

	if token == "" {
		return ErrTokenRequired
	}

	result, err := s.callSiteverify(ctx, token, remoteIP)
	if err != nil {
		if s.failOpen {
			slog.WarnContext(ctx, "captcha verify failed open",
				slog.String("provider", string(s.provider)),
				slog.String("error", err.Error()),
			)

			return nil
		}

		return api.WrapHTTPError(
			errors.WithMessage(err, "captcha verification unavailable"),
			http.StatusServiceUnavailable,
		)
	}

	if !result.Success {
		slog.WarnContext(ctx, "captcha rejected",
			slog.String("provider", string(s.provider)),
			slog.Any("error_codes", result.ErrorCodes),
		)

		return ErrVerificationFailed
	}

	if s.provider == ProviderReCAPTCHAV3 && result.Score < s.minScore {
		slog.WarnContext(ctx, "captcha score below threshold",
			slog.Float64("score", result.Score),
			slog.Float64("min_score", s.minScore),
		)

		return ErrVerificationFailed
	}

	return nil
}

func (s *Service) callSiteverify(
	ctx context.Context, token, remoteIP string,
) (*verifyResponse, error) {
	form := url.Values{}
	form.Set("secret", s.secretKey)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, s.verifyURL, strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req) //nolint:bodyclose
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute request")
	}
	defer func(body io.ReadCloser) {
		if cerr := body.Close(); cerr != nil {
			slog.Warn("failed to close response body", "error", cerr)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("unexpected HTTP status code: %d", resp.StatusCode)
	}

	var result verifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.Wrap(err, "failed to decode response")
	}

	return &result, nil
}
