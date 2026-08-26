package netutil

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveBindAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		host     string
		wantAddr string
	}{
		{
			name:     "empty_host_returns_empty",
			host:     "",
			wantAddr: "",
		},
		{
			name:     "wildcard_ipv4_returns_as_is",
			host:     "0.0.0.0",
			wantAddr: "0.0.0.0",
		},
		{
			name:     "wildcard_ipv6_returns_as_is",
			host:     "::",
			wantAddr: "::",
		},
		{
			name:     "explicit_ipv4_returns_as_is",
			host:     "192.168.1.1",
			wantAddr: "192.168.1.1",
		},
		{
			name:     "explicit_ipv6_returns_as_is",
			host:     "::1",
			wantAddr: "::1",
		},
		{
			name:     "explicit_ipv4_loopback_returns_as_is",
			host:     "127.0.0.1",
			wantAddr: "127.0.0.1",
		},
		{
			name:     "unresolvable_domain_returns_wildcard",
			host:     "nonexistent.domain.invalid",
			wantAddr: "0.0.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ResolveBindAddress(context.Background(), tt.host)
			assert.Equal(t, tt.wantAddr, got)
		})
	}
}

func TestResolveBindAddress_localhost(t *testing.T) {
	t.Parallel()

	result := ResolveBindAddress(context.Background(), "localhost")

	parsed := net.ParseIP(result)
	assert.NotNil(t, parsed, "expected a valid IP, got %q", result)
	assert.True(t, parsed.IsLoopback(), "expected loopback address, got %s", result)
}
