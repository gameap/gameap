package dnsprovider_test

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/acme/dnsprovider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cloudflareEnvVars are every Cloudflare credential environment variable lego
// recognises. We must clear all of them in tests that exercise the
// "no credentials" failure path because the host machine may have one set.
//
// Source: github.com/go-acme/lego/v4/providers/dns/cloudflare/cloudflare.go .
var cloudflareEnvVars = []string{
	"CLOUDFLARE_DNS_API_TOKEN",
	"CLOUDFLARE_API_TOKEN",
	"CF_DNS_API_TOKEN",
	"CF_API_TOKEN",
	"CLOUDFLARE_API_KEY",
	"CF_API_KEY",
	"CLOUDFLARE_EMAIL",
	"CF_API_EMAIL",
	"CLOUDFLARE_ZONE_API_TOKEN",
	"CF_ZONE_API_TOKEN",
}

func clearCloudflareEnv(t *testing.T) {
	t.Helper()

	for _, v := range cloudflareEnvVars {
		t.Setenv(v, "")
	}
}

//nolint:paralleltest // uses t.Setenv to mutate Cloudflare credential environment variables
func TestBuiltinRegistry_Resolve(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		setupEnv   func(*testing.T)
		wantError  string
	}{
		{
			name:       "empty_identifier_rejected",
			identifier: "",
			setupEnv:   clearCloudflareEnv,
			wantError:  "identifier is empty",
		},
		{
			name:       "plugin_prefixed_identifier_rejected",
			identifier: "myplugin:cloudflare",
			setupEnv:   clearCloudflareEnv,
			wantError:  "plugin prefix",
		},
		{
			name:       "unknown_provider_rejected",
			identifier: "route53",
			setupEnv:   clearCloudflareEnv,
			wantError:  "is not registered",
		},
		{
			name:       "cloudflare_without_credentials_returns_error",
			identifier: "cloudflare",
			setupEnv:   clearCloudflareEnv,
			wantError:  "failed to construct cloudflare DNS provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			tt.setupEnv(t)
			r := dnsprovider.NewBuiltinRegistry()

			// ACT
			provider, err := r.Resolve(context.Background(), tt.identifier)

			// ASSERT
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			assert.Nil(t, provider, "no provider must be returned on error")
		})
	}
}

func TestBuiltinRegistry_Resolve_CloudflareWithToken(t *testing.T) {
	// ARRANGE: lego validates only the presence of credentials, not their
	// validity at construction time. A non-empty placeholder is enough to
	// build the provider; network calls happen later in Present/CleanUp.
	clearCloudflareEnv(t)
	t.Setenv("CLOUDFLARE_DNS_API_TOKEN", "fake-token-1234567890abcdef")

	r := dnsprovider.NewBuiltinRegistry()

	// ACT
	provider, err := r.Resolve(context.Background(), "cloudflare")

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, provider, "cloudflare provider must be constructed when token is present")
}

func TestBuiltinRegistry_Resolve_PluginPrefixDetectedRegardlessOfName(t *testing.T) {
	t.Parallel()

	// ARRANGE: any identifier containing ":" must be rejected as plugin-prefixed,
	// even if the suffix is a valid built-in name.
	tests := []string{
		"plugin:cloudflare",
		"any:provider",
		":empty-prefix",
		"cloudflare:",
	}

	for _, id := range tests {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			r := dnsprovider.NewBuiltinRegistry()

			provider, err := r.Resolve(context.Background(), id)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "plugin prefix")
			assert.Nil(t, provider)
		})
	}
}
