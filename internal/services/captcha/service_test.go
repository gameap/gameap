package captcha

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStubServer(t *testing.T, status int, body any, capture *url.Values) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		if capture != nil {
			*capture = r.PostForm
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)

		if body == nil {
			return
		}

		if s, ok := body.(string); ok {
			_, _ = w.Write([]byte(s))

			return
		}

		_ = json.NewEncoder(w).Encode(body)
	}))
}

func TestService_Enabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			name: "disabled_when_provider_empty",
			cfg:  Config{Provider: ProviderNone, SecretKey: "secret"},
			want: false,
		},
		{
			name: "disabled_when_secret_missing",
			cfg:  Config{Provider: ProviderTurnstile},
			want: false,
		},
		{
			name: "enabled_turnstile",
			cfg:  Config{Provider: ProviderTurnstile, SecretKey: "secret"},
			want: true,
		},
		{
			name: "enabled_recaptcha_v2",
			cfg:  Config{Provider: ProviderReCAPTCHAV2, SecretKey: "secret"},
			want: true,
		},
		{
			name: "enabled_fcaptcha",
			cfg: Config{
				Provider: ProviderFCaptcha, SecretKey: "secret", InstanceURL: "https://captcha.example.com",
			},
			want: true,
		},
		{
			name: "disabled_fcaptcha_without_instance_url",
			cfg:  Config{Provider: ProviderFCaptcha, SecretKey: "secret"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewService(tt.cfg)

			assert.Equal(t, tt.want, s.Enabled())
		})
	}
}

func TestService_Verify(t *testing.T) {
	tests := []struct {
		name         string
		cfg          Config
		token        string
		serverStatus int
		serverBody   any
		noServer     bool
		wantError    string
	}{
		{
			name:      "disabled_provider_is_noop_even_without_token",
			cfg:       Config{Provider: ProviderNone},
			token:     "",
			noServer:  true,
			wantError: "",
		},
		{
			name:      "missing_token_rejected",
			cfg:       Config{Provider: ProviderTurnstile, SecretKey: "secret"},
			token:     "",
			noServer:  true,
			wantError: "captcha token is required",
		},
		{
			name:         "recaptcha_v2_success",
			cfg:          Config{Provider: ProviderReCAPTCHAV2, SecretKey: "secret"},
			token:        "tok",
			serverStatus: http.StatusOK,
			serverBody:   verifyResponse{Success: true},
			wantError:    "",
		},
		{
			name:         "turnstile_success",
			cfg:          Config{Provider: ProviderTurnstile, SecretKey: "secret"},
			token:        "tok",
			serverStatus: http.StatusOK,
			serverBody:   verifyResponse{Success: true},
			wantError:    "",
		},
		{
			name: "fcaptcha_success",
			cfg: Config{
				Provider: ProviderFCaptcha, SecretKey: "secret", InstanceURL: "https://captcha.example.com",
			},
			token:        "tok",
			serverStatus: http.StatusOK,
			serverBody:   verifyResponse{Success: true},
			wantError:    "",
		},
		{
			name:         "recaptcha_v3_above_threshold_passes",
			cfg:          Config{Provider: ProviderReCAPTCHAV3, SecretKey: "secret", MinScore: 0.5},
			token:        "tok",
			serverStatus: http.StatusOK,
			serverBody:   verifyResponse{Success: true, Score: 0.9},
			wantError:    "",
		},
		{
			name:         "recaptcha_v3_below_threshold_rejected",
			cfg:          Config{Provider: ProviderReCAPTCHAV3, SecretKey: "secret", MinScore: 0.5},
			token:        "tok",
			serverStatus: http.StatusOK,
			serverBody:   verifyResponse{Success: true, Score: 0.1},
			wantError:    "captcha verification failed",
		},
		{
			name:         "provider_rejects_token",
			cfg:          Config{Provider: ProviderTurnstile, SecretKey: "secret"},
			token:        "bad",
			serverStatus: http.StatusOK,
			serverBody:   verifyResponse{Success: false, ErrorCodes: []string{"invalid-input-response"}},
			wantError:    "captcha verification failed",
		},
		{
			name:         "upstream_error_closed_blocks_login",
			cfg:          Config{Provider: ProviderTurnstile, SecretKey: "secret", FailOpen: false},
			token:        "tok",
			serverStatus: http.StatusInternalServerError,
			serverBody:   "boom",
			wantError:    "captcha verification unavailable",
		},
		{
			name:         "upstream_error_open_allows_login",
			cfg:          Config{Provider: ProviderTurnstile, SecretKey: "secret", FailOpen: true},
			token:        "tok",
			serverStatus: http.StatusInternalServerError,
			serverBody:   "boom",
			wantError:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []Option{}
			if !tt.noServer {
				server := newStubServer(t, tt.serverStatus, tt.serverBody, nil)
				defer server.Close()
				opts = append(opts, WithVerifyURL(server.URL))
			}

			s := NewService(tt.cfg, opts...)

			err := s.Verify(context.Background(), tt.token, "")

			if tt.wantError == "" {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

func TestService_Verify_ForwardsSecretAndRemoteIP(t *testing.T) {
	var captured url.Values

	server := newStubServer(t, http.StatusOK, verifyResponse{Success: true}, &captured)
	defer server.Close()

	s := NewService(
		Config{Provider: ProviderTurnstile, SecretKey: "the-secret"},
		WithVerifyURL(server.URL),
	)

	err := s.Verify(context.Background(), "the-token", "203.0.113.7")
	require.NoError(t, err)

	assert.Equal(t, "the-secret", captured.Get("secret"))
	assert.Equal(t, "the-token", captured.Get("response"))
	assert.Equal(t, "203.0.113.7", captured.Get("remoteip"))
}

func TestResolveVerifyURL(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		override string
		instance string
		want     string
	}{
		{
			name:     "turnstile_default_endpoint",
			provider: ProviderTurnstile,
			want:     "https://challenges.cloudflare.com/turnstile/v0/siteverify",
		},
		{
			name:     "recaptcha_v2_default_endpoint",
			provider: ProviderReCAPTCHAV2,
			want:     "https://www.google.com/recaptcha/api/siteverify",
		},
		{
			name:     "recaptcha_v3_default_endpoint",
			provider: ProviderReCAPTCHAV3,
			want:     "https://www.google.com/recaptcha/api/siteverify",
		},
		{
			name:     "fcaptcha_uses_instance_endpoint",
			provider: ProviderFCaptcha,
			instance: "https://captcha.example.com/base/",
			want:     "https://captcha.example.com/base/turnstile/v0/siteverify",
		},
		{
			name:     "fcaptcha_rejects_instance_query",
			provider: ProviderFCaptcha,
			instance: "https://captcha.example.com?tenant=1",
			want:     "",
		},
		{
			name:     "fcaptcha_rejects_non_http_instance",
			provider: ProviderFCaptcha,
			instance: "javascript:alert(1)",
			want:     "",
		},
		{
			name:     "fcaptcha_without_instance_has_no_endpoint",
			provider: ProviderFCaptcha,
			want:     "",
		},
		{
			name:     "provider_none_has_no_endpoint",
			provider: ProviderNone,
			want:     "",
		},
		{
			name:     "unknown_provider_has_no_endpoint",
			provider: Provider("totally-unknown"),
			want:     "",
		},
		{
			name:     "override_wins_over_turnstile_default",
			provider: ProviderTurnstile,
			override: "https://proxy.example/verify",
			want:     "https://proxy.example/verify",
		},
		{
			name:     "override_wins_over_recaptcha_default",
			provider: ProviderReCAPTCHAV3,
			override: "https://proxy.example/verify",
			want:     "https://proxy.example/verify",
		},
		{
			name:     "override_wins_even_for_unknown_provider",
			provider: Provider("totally-unknown"),
			override: "https://proxy.example/verify",
			want:     "https://proxy.example/verify",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			got := resolveVerifyURL(tt.provider, tt.override, tt.instance)

			// ASSERT
			assert.Equal(t, tt.want, got, "resolved siteverify endpoint mismatch")
		})
	}
}

// TestService_Enabled_VerifyURLGate complements TestService_Enabled with the
// verifyURL leg of the predicate: a provider+secret pair is still disabled
// when no endpoint could be resolved (unknown provider, no override), and a
// non-empty override revives an otherwise-unroutable provider.
func TestService_Enabled_VerifyURLGate(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			name: "enabled_recaptcha_v3_with_secret",
			cfg:  Config{Provider: ProviderReCAPTCHAV3, SecretKey: "secret"},
			want: true,
		},
		{
			name: "disabled_when_verify_url_unresolved_for_unknown_provider",
			cfg:  Config{Provider: Provider("mystery"), SecretKey: "secret"},
			want: false,
		},
		{
			name: "enabled_when_override_supplies_endpoint_for_unknown_provider",
			cfg:  Config{Provider: Provider("mystery"), SecretKey: "secret", VerifyURL: "https://proxy.example/verify"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE / ACT
			s := NewService(tt.cfg)

			// ASSERT
			assert.Equal(t, tt.want, s.Enabled(), "Enabled() must also gate on a resolved verifyURL")
		})
	}
}

// TestService_Verify_UpstreamFailures covers every transport/parse failure
// path of callSiteverify, each split into fail-closed (login blocked with a
// 503-mapped "captcha verification unavailable") and FailOpen (login waved
// through, nil error).
func TestService_Verify_UpstreamFailures(t *testing.T) {
	tests := []struct {
		name string
		// serverURL builds the verifyURL for the run; for the transport-error
		// case it returns the URL of an already-closed server.
		serverURL func(t *testing.T) string
		failOpen  bool
		wantError string
	}{
		{
			name: "transport_error_closed_blocks_login",
			serverURL: func(t *testing.T) string {
				t.Helper()
				closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
				u := closed.URL
				closed.Close()

				return u
			},
			failOpen:  false,
			wantError: "captcha verification unavailable",
		},
		{
			name: "transport_error_open_allows_login",
			serverURL: func(t *testing.T) string {
				t.Helper()
				closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
				u := closed.URL
				closed.Close()

				return u
			},
			failOpen:  true,
			wantError: "",
		},
		{
			name: "request_create_error_closed_blocks_login",
			serverURL: func(t *testing.T) string {
				t.Helper()

				return "http://%zz"
			},
			failOpen:  false,
			wantError: "captcha verification unavailable",
		},
		{
			name: "request_create_error_open_allows_login",
			serverURL: func(t *testing.T) string {
				t.Helper()

				return "http://%zz"
			},
			failOpen:  true,
			wantError: "",
		},
		{
			name: "json_decode_error_closed_blocks_login",
			serverURL: func(t *testing.T) string {
				t.Helper()
				srv := newStubServer(t, http.StatusOK, "not json", nil)
				t.Cleanup(srv.Close)

				return srv.URL
			},
			failOpen:  false,
			wantError: "captcha verification unavailable",
		},
		{
			name: "json_decode_error_open_allows_login",
			serverURL: func(t *testing.T) string {
				t.Helper()
				srv := newStubServer(t, http.StatusOK, "not json", nil)
				t.Cleanup(srv.Close)

				return srv.URL
			},
			failOpen:  true,
			wantError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			s := NewService(
				Config{Provider: ProviderTurnstile, SecretKey: "secret", FailOpen: tt.failOpen},
				WithVerifyURL(tt.serverURL(t)),
			)

			// ACT
			err := s.Verify(context.Background(), "tok", "")

			// ASSERT
			if tt.wantError == "" {
				require.NoError(t, err, "FailOpen must wave the login through on upstream failure")

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError, "fail-closed must surface an unavailable error")
		})
	}
}

// TestService_Verify_ScoreBoundary pins the reCAPTCHA v3 score gate: the
// comparison is strict less-than, so score == minScore passes; a zero
// threshold accepts a zero score; and a non-v3 provider ignores the score
// field entirely.
func TestService_Verify_ScoreBoundary(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		body      verifyResponse
		wantError string
	}{
		{
			name:      "v3_score_equal_to_min_passes",
			cfg:       Config{Provider: ProviderReCAPTCHAV3, SecretKey: "secret", MinScore: 0.5},
			body:      verifyResponse{Success: true, Score: 0.5},
			wantError: "",
		},
		{
			name:      "v3_zero_min_score_accepts_zero_score",
			cfg:       Config{Provider: ProviderReCAPTCHAV3, SecretKey: "secret", MinScore: 0},
			body:      verifyResponse{Success: true, Score: 0},
			wantError: "",
		},
		{
			name:      "turnstile_ignores_zero_score_when_success",
			cfg:       Config{Provider: ProviderTurnstile, SecretKey: "secret", MinScore: 0.9},
			body:      verifyResponse{Success: true, Score: 0},
			wantError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			server := newStubServer(t, http.StatusOK, tt.body, nil)
			defer server.Close()
			s := NewService(tt.cfg, WithVerifyURL(server.URL))

			// ACT
			err := s.Verify(context.Background(), "tok", "")

			// ASSERT
			if tt.wantError == "" {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

// TestService_Verify_RequestShape asserts the wire contract callSiteverify
// must keep: POST, the two fixed headers, and that remoteip is omitted when
// no client IP is known (negative of TestService_Verify_ForwardsSecretAndRemoteIP).
func TestService_Verify_RequestShape(t *testing.T) {
	var (
		gotMethod      string
		gotContentType string
		gotAccept      string
		gotForm        url.Values
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		gotForm = r.PostForm

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(verifyResponse{Success: true})
	}))
	defer server.Close()

	s := NewService(
		Config{Provider: ProviderTurnstile, SecretKey: "secret"},
		WithVerifyURL(server.URL),
	)

	// ACT
	err := s.Verify(context.Background(), "tok", "")

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, gotMethod, "siteverify must be a POST")
	assert.Equal(t, "application/x-www-form-urlencoded", gotContentType, "form encoding header")
	assert.Equal(t, "application/json", gotAccept, "Accept header must request JSON")
	assert.Empty(t, gotForm.Get("remoteip"),
		"remoteip must be absent when no client IP is known")
	assert.Equal(t, "tok", gotForm.Get("response"),
		"the token must still be forwarded without a remote IP")
}

// TestService_Verify_RejectsRecaptchaV2 mirrors the existing turnstile
// rejection case for reCAPTCHA v2: success=false yields the 422 verification
// error regardless of provider.
func TestService_Verify_RejectsRecaptchaV2(t *testing.T) {
	// ARRANGE
	server := newStubServer(
		t, http.StatusOK,
		verifyResponse{Success: false, ErrorCodes: []string{"invalid-input-response"}}, nil,
	)
	defer server.Close()
	s := NewService(
		Config{Provider: ProviderReCAPTCHAV2, SecretKey: "secret"},
		WithVerifyURL(server.URL),
	)

	// ACT
	err := s.Verify(context.Background(), "bad", "")

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "captcha verification failed",
		"a rejected reCAPTCHA v2 token must be a verification failure")
}

var errForcedTransport = errors.New("forced transport boom")

// recordingRoundTripper is an http.RoundTripper test double proving Verify
// drives the injected *http.Client rather than its own default client.
type recordingRoundTripper struct {
	called bool
	err    error
	resp   *http.Response
}

func (rt *recordingRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	rt.called = true
	if rt.err != nil {
		return nil, rt.err
	}

	return rt.resp, nil
}

// TestService_Verify_UsesInjectedHTTPClient asserts WithHTTPClient is wired:
// a custom transport returning a forced error must be the thing Verify calls,
// so the error propagates through the fail-closed path.
func TestService_Verify_UsesInjectedHTTPClient(t *testing.T) {
	// ARRANGE
	rt := &recordingRoundTripper{err: errForcedTransport}
	s := NewService(
		Config{Provider: ProviderTurnstile, SecretKey: "secret"},
		WithVerifyURL("https://verify.invalid/siteverify"),
		WithHTTPClient(&http.Client{Transport: rt}),
	)

	// ACT
	err := s.Verify(context.Background(), "tok", "")

	// ASSERT
	require.True(t, rt.called, "Verify must dispatch through the injected http.Client transport")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "captcha verification unavailable",
		"the injected transport's error must propagate fail-closed")
}
