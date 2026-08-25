package enrollment

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: ""},
		{name: "whitespace_only", raw: "   ", want: ""},
		{name: "hostname", raw: "panel.example.com", want: "panel.example.com"},
		{name: "hostname_with_port", raw: "panel.example.com:8080", want: "panel.example.com"},
		{name: "https_scheme_with_port", raw: "https://panel.example.com:8080", want: "panel.example.com"},
		{name: "http_scheme", raw: "http://panel.example.com", want: "panel.example.com"},
		{name: "ipv4", raw: "203.0.113.10", want: "203.0.113.10"},
		{name: "ipv4_with_port", raw: "203.0.113.10:31717", want: "203.0.113.10"},
		{name: "bare_ipv6", raw: "2001:db8::1", want: "2001:db8::1"},
		{name: "bracketed_ipv6", raw: "[2001:db8::1]", want: "2001:db8::1"},
		{name: "bracketed_ipv6_with_port", raw: "[2001:db8::1]:31718", want: "2001:db8::1"},
		{name: "bracketed_ipv6_with_scheme", raw: "https://[2001:db8::1]:8080", want: "2001:db8::1"},
		{name: "ipv6_loopback", raw: "::1", want: "::1"},
		{name: "bracketed_ipv6_loopback_with_port", raw: "[::1]:31717", want: "::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, NormalizeHost(tt.raw))
		})
	}
}

// TestConnectResolver_IPv6RoundTrip pins the whole path a daemon address takes:
// the plugin-supplied authority is normalized, formatted into a connect URL and
// parsed back by the daemon side.
func TestConnectResolver_IPv6RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fallbackHost string
		wantHost     string
		wantURL      string
	}{
		{
			name:         "bracketed_ipv6_with_port",
			fallbackHost: "[2001:db8::1]:31718",
			wantHost:     "2001:db8::1",
			wantURL:      "grpc://[2001:db8::1]:31717/setup-key",
		},
		{
			name:         "bare_ipv6",
			fallbackHost: "2001:db8::1",
			wantHost:     "2001:db8::1",
			wantURL:      "grpc://[2001:db8::1]:31717/setup-key",
		},
		{
			name:         "hostname",
			fallbackHost: "panel.example.com:8080",
			wantHost:     "panel.example.com",
			wantURL:      "grpc://panel.example.com:31717/setup-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolver := NewConnectResolver("", 31717, 0, nil)

			target, err := resolver.Resolve(tt.fallbackHost)
			require.NoError(t, err)
			assert.Equal(t, tt.wantHost, target.Host)

			rawURL := FormatConnectURL(target.Host, target.Port, "setup-key")
			assert.Equal(t, tt.wantURL, rawURL)

			info, err := ParseConnectURL(rawURL)
			require.NoError(t, err)
			assert.Equal(t, tt.wantHost, info.Host)
			assert.Equal(t, uint16(31717), info.Port)
			assert.Equal(t, "setup-key", info.SetupKey)
		})
	}
}
