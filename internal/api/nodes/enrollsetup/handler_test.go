package enrollsetup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/enrollment"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupKeyManager(t *testing.T, setupKey string) *enrollment.SetupKeyManager {
	t.Helper()

	cacheInstance := cache.NewInMemory()
	m := enrollment.NewSetupKeyManager(cacheInstance, "")

	if setupKey != "" {
		err := m.Set(context.Background(), setupKey)
		require.NoError(t, err)
	}

	return m
}

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		key            string
		setupKey       string
		grpcExtHost    string
		grpcPort       uint16
		grpcExtPort    uint16
		panelHost      string
		requestHost    string
		config         string
		github         string
		branch         string
		expectedStatus int
		wantError      string
		validateResp   func(*testing.T, string)
	}{
		{
			name:           "successful_setup_returns_script",
			key:            "AbCdEfGh1234567890AbCdEfGh123456",
			setupKey:       "AbCdEfGh1234567890AbCdEfGh123456",
			grpcPort:       31718,
			panelHost:      "panel.example.com",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp string) {
				t.Helper()

				assert.Contains(t, resp, "#!/bin/bash")
				assert.Contains(t, resp, "set -euo pipefail")
				assert.Contains(t, resp, "trap cleanup EXIT")
				// Shell-escaped (single-quoted) so a header-derived host cannot inject commands.
				assert.Contains(t, resp, "CONNECT_URL='grpc://panel.example.com:31718/AbCdEfGh1234567890AbCdEfGh123456'")
				assert.Contains(t, resp, "$(id -u)")
				assert.Contains(t, resp, "command -v gameapctl")
				assert.Contains(t, resp, "gameapctl self-update")
				assert.Contains(t, resp, "command -v \"$cmd\"")
				assert.Contains(t, resp, "curl -sLf")
				assert.Contains(t, resp, "api.github.com/repos/gameap/gameapctl/releases")
				assert.Contains(t, resp, "mktemp -t gameapctl.XXXXXX")
				assert.Contains(t, resp, "install -m 0755 \"${_tmpbin}\" /usr/local/bin/gameapctl")
				assert.Contains(t, resp, "\"$GAMEAPCTL_BIN\" daemon install --connect=\"$CONNECT_URL\"")
				assert.Contains(t, resp, "\"$@\"")
			},
		},
		{
			name:           "uses_external_host_and_port",
			key:            "AbCdEfGh1234567890AbCdEfGh123456",
			setupKey:       "AbCdEfGh1234567890AbCdEfGh123456",
			grpcExtHost:    "external.example.com",
			grpcPort:       31718,
			grpcExtPort:    9090,
			panelHost:      "panel.example.com",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp string) {
				t.Helper()

				assert.Contains(t, resp, "grpc://external.example.com:9090/")
			},
		},
		{
			name:           "with_config_parameter",
			key:            "AbCdEfGh1234567890AbCdEfGh123456",
			setupKey:       "AbCdEfGh1234567890AbCdEfGh123456",
			grpcPort:       31718,
			panelHost:      "panel.example.com",
			config:         "process_manager.name=systemd",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp string) {
				t.Helper()

				assert.Contains(t, resp, "--config='process_manager.name=systemd'")
			},
		},
		{
			name:           "with_github_parameter",
			key:            "AbCdEfGh1234567890AbCdEfGh123456",
			setupKey:       "AbCdEfGh1234567890AbCdEfGh123456",
			grpcPort:       31718,
			panelHost:      "panel.example.com",
			github:         "true",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp string) {
				t.Helper()

				assert.Contains(t, resp, " --github")
			},
		},
		{
			name:           "with_branch_parameter",
			key:            "AbCdEfGh1234567890AbCdEfGh123456",
			setupKey:       "AbCdEfGh1234567890AbCdEfGh123456",
			grpcPort:       31718,
			panelHost:      "panel.example.com",
			branch:         "develop",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp string) {
				t.Helper()

				assert.Contains(t, resp, "--branch='develop'")
			},
		},
		{
			name:           "with_all_parameters",
			key:            "AbCdEfGh1234567890AbCdEfGh123456",
			setupKey:       "AbCdEfGh1234567890AbCdEfGh123456",
			grpcPort:       31718,
			panelHost:      "panel.example.com",
			config:         "process_manager.name=systemd",
			github:         "true",
			branch:         "develop",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp string) {
				t.Helper()

				assert.Contains(t, resp, "--config='process_manager.name=systemd'")
				assert.Contains(t, resp, " --github")
				assert.Contains(t, resp, "--branch='develop'")
			},
		},
		{
			name:           "passes_through_cli_args",
			key:            "AbCdEfGh1234567890AbCdEfGh123456",
			setupKey:       "AbCdEfGh1234567890AbCdEfGh123456",
			grpcPort:       31718,
			panelHost:      "panel.example.com",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp string) {
				t.Helper()

				assert.Contains(t, resp, "\"$GAMEAPCTL_BIN\" daemon install --connect=\"$CONNECT_URL\" \"$@\"")
			},
		},
		{
			name:           "shell_escape_in_config",
			key:            "AbCdEfGh1234567890AbCdEfGh123456",
			setupKey:       "AbCdEfGh1234567890AbCdEfGh123456",
			grpcPort:       31718,
			panelHost:      "panel.example.com",
			config:         "val=$(whoami)",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp string) {
				t.Helper()

				assert.Contains(t, resp, "--config='val=$(whoami)'")
				assert.NotContains(t, resp, "--config=\"val=$(whoami)\"")
			},
		},
		{
			name:           "invalid_key_returns_forbidden",
			key:            "wrong-key",
			setupKey:       "AbCdEfGh1234567890AbCdEfGh123456",
			grpcPort:       31718,
			panelHost:      "panel.example.com",
			expectedStatus: http.StatusForbidden,
			wantError:      "invalid setup key",
		},
		{
			name:           "no_setup_key_configured_returns_forbidden",
			key:            "some-key",
			setupKey:       "",
			grpcPort:       31718,
			panelHost:      "panel.example.com",
			expectedStatus: http.StatusForbidden,
			wantError:      "invalid setup key",
		},
		{
			name:           "detects_host_from_request",
			key:            "AbCdEfGh1234567890AbCdEfGh123456",
			setupKey:       "AbCdEfGh1234567890AbCdEfGh123456",
			grpcPort:       31718,
			panelHost:      "",
			requestHost:    "detected.example.com:8080",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp string) {
				t.Helper()

				assert.Contains(t, resp, "grpc://detected.example.com:31718/")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			keyManager := setupKeyManager(t, tt.setupKey)
			cacheInstance := cache.NewInMemory()

			if tt.setupKey != "" {
				err := cacheInstance.Set(context.Background(), enrollment.SetupKeyCacheKey, tt.setupKey)
				require.NoError(t, err)
			}

			enrollSvc := enrollment.NewService(
				keyManager,
				nil, nil, nil,
			)

			responder := api.NewResponder()
			handler := NewHandler(
				enrollSvc,
				responder,
				tt.panelHost,
				tt.grpcExtHost,
				tt.grpcPort,
				tt.grpcExtPort,
				nil,
			)

			path := "/nodes/setup/" + tt.key
			params := ""
			if tt.config != "" {
				params += "&config=" + tt.config
			}
			if tt.github != "" {
				params += "&github=" + tt.github
			}
			if tt.branch != "" {
				params += "&branch=" + tt.branch
			}
			if params != "" {
				path += "?" + params[1:]
			}

			req := httptest.NewRequest(http.MethodGet, path, nil)
			if tt.requestHost != "" {
				req.Host = tt.requestHost
			}

			req = mux.SetURLVars(req, map[string]string{"key": tt.key})

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			body := w.Body.String()

			if tt.wantError != "" {
				assert.Contains(t, body, tt.wantError)
			}

			if tt.validateResp != nil {
				tt.validateResp(t, body)
			}
		})
	}
}

// TestHandler_ServeHTTP_ShellInjectionSafety pins the command-injection
// defence of the generated root-run setup script.
//
// OWASP API Security Top 10:2023:
//   - API8:2023 Security Misconfiguration — every request-controlled value that
//     lands in the script (the connect-URL host derived from the Host /
//     X-Forwarded-Host header, and the config / branch query parameters) must
//     be POSIX single-quote escaped, otherwise a crafted value could break out
//     of its argument and execute arbitrary commands as root on the node. A
//     single quote in the payload must be neutralised by the close-escape-reopen
//     sequence, and every other metacharacter stays inert inside the quotes.
//
// Reference: https://owasp.org/API-Security/editions/2023/
func TestHandler_ServeHTTP_ShellInjectionSafety(t *testing.T) {
	t.Parallel()
	const key = "AbCdEfGh1234567890AbCdEfGh123456"

	tests := []struct {
		name            string
		panelHost       string
		requestHost     string
		config          string
		branch          string
		wantContains    []string
		wantNotContains []string
	}{
		{
			// A single quote in the header-derived host must be neutralised
			// inside the single-quoted CONNECT_URL assignment.
			name:        "single_quote_in_host_is_neutralised",
			panelHost:   "",
			requestHost: "evil'.example.com",
			wantContains: []string{
				`CONNECT_URL='grpc://evil'\''.example.com:31718/AbCdEfGh1234567890AbCdEfGh123456'`,
			},
		},
		{
			// Command substitution and a statement separator stay inert because
			// the whole value is wrapped in single quotes.
			name:      "substitution_and_semicolon_in_config_stay_quoted",
			panelHost: "panel.example.com",
			config:    "x=$(id);rm -rf /",
			wantContains: []string{
				`--config='x=$(id);rm -rf /'`,
			},
			wantNotContains: []string{
				`--config=x=$(id)`, // never emitted unquoted
			},
		},
		{
			name:      "single_quote_in_config_is_neutralised",
			panelHost: "panel.example.com",
			config:    "a'b",
			wantContains: []string{
				`--config='a'\''b'`,
			},
		},
		{
			// Backticks are literal inside single quotes, so command
			// substitution via backticks cannot fire.
			name:      "backtick_in_branch_stays_quoted",
			panelHost: "panel.example.com",
			branch:    "`id`",
			wantContains: []string{
				"--branch='`id`'",
			},
			wantNotContains: []string{
				"--branch=`id`",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			keyManager := setupKeyManager(t, key)
			enrollSvc := enrollment.NewService(keyManager, nil, nil, nil)
			handler := NewHandler(
				enrollSvc,
				api.NewResponder(),
				tt.panelHost,
				"", // grpcExtHost unset so the header-derived host is used
				31718,
				0,
				nil,
			)

			q := url.Values{}
			if tt.config != "" {
				q.Set("config", tt.config)
			}
			if tt.branch != "" {
				q.Set("branch", tt.branch)
			}
			target := "/nodes/setup/" + key
			if enc := q.Encode(); enc != "" {
				target += "?" + enc
			}

			req := httptest.NewRequest(http.MethodGet, target, nil)
			if tt.requestHost != "" {
				req.Host = tt.requestHost
			}
			req = mux.SetURLVars(req, map[string]string{"key": key})

			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
			body := w.Body.String()

			for _, want := range tt.wantContains {
				assert.Contains(t, body, want,
					"the request-controlled value must appear only in its POSIX-escaped form")
			}
			for _, notWant := range tt.wantNotContains {
				assert.NotContains(t, body, notWant,
					"the value must never appear unquoted (that would allow injection)")
			}
		})
	}
}

func TestHandler_GRPCDisabled(t *testing.T) {
	t.Parallel()
	responder := api.NewResponder()
	handler := NewHandler(nil, responder, "", "", 0, 0, nil)

	req := httptest.NewRequest(http.MethodGet, "/nodes/setup/somekey", nil)
	req = mux.SetURLVars(req, map[string]string{"key": "somekey"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// The uncovered-host warning is log-only for this endpoint: the generated
// script must stay byte-identical so existing installs are not disturbed.
func TestHandler_ServeHTTP_uncoveredHostKeepsScriptUnchanged(t *testing.T) {
	t.Parallel()
	const key = "AbCdEfGh1234567890AbCdEfGh123456"

	buildScript := func(certHostCovered func(string) bool) string {
		keyManager := setupKeyManager(t, key)
		enrollSvc := enrollment.NewService(keyManager, nil, nil, nil)
		handler := NewHandler(enrollSvc, api.NewResponder(), "panel.example.com", "", 31718, 0, certHostCovered)

		req := httptest.NewRequest(http.MethodGet, "/nodes/setup/"+key, nil)
		req = mux.SetURLVars(req, map[string]string{"key": key})
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		return w.Body.String()
	}

	covered := buildScript(func(string) bool { return true })
	uncovered := buildScript(func(string) bool { return false })

	assert.Equal(t, covered, uncovered)
	assert.Contains(t, uncovered, "grpc://panel.example.com:31718/"+key)
}
