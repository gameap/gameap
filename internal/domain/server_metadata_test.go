package domain_test

import (
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestServer_PublicIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		server   domain.Server
		wantIP   string
		wantHTTP string
	}{
		{
			name:   "nil_metadata",
			server: domain.Server{},
			wantIP: "",
		},
		{
			name:   "empty_metadata",
			server: domain.Server{Metadata: domain.Metadata{}},
			wantIP: "",
		},
		{
			name:   "other_keys_only",
			server: domain.Server{Metadata: domain.Metadata{"docker_image": "gameap/debian"}},
			wantIP: "",
		},
		{
			name:   "empty_string_value",
			server: domain.Server{Metadata: domain.Metadata{"public_ip": ""}},
			wantIP: "",
		},
		{
			name:   "blank_string_value",
			server: domain.Server{Metadata: domain.Metadata{"public_ip": "   "}},
			wantIP: "",
		},
		{
			name:   "non_string_value",
			server: domain.Server{Metadata: domain.Metadata{"public_ip": 12345}},
			wantIP: "",
		},
		{
			name:   "ipv4",
			server: domain.Server{Metadata: domain.Metadata{"public_ip": "203.0.113.10"}},
			wantIP: "203.0.113.10",
		},
		{
			name:   "hostname",
			server: domain.Server{Metadata: domain.Metadata{"public_ip": "play.example.com"}},
			wantIP: "play.example.com",
		},
		{
			name:   "surrounding_whitespace_trimmed",
			server: domain.Server{Metadata: domain.Metadata{"public_ip": " 203.0.113.10 "}},
			wantIP: "203.0.113.10",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.wantIP, test.server.PublicIP())
		})
	}
}

func TestServer_VisibleServerIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		server domain.Server
		want   string
	}{
		{
			name:   "public_ip_wins_when_set",
			server: domain.Server{ServerIP: "10.0.0.5", Metadata: domain.Metadata{"public_ip": "203.0.113.10"}},
			want:   "203.0.113.10",
		},
		{
			name:   "real_ip_without_metadata",
			server: domain.Server{ServerIP: "10.0.0.5"},
			want:   "10.0.0.5",
		},
		{
			name:   "real_ip_when_public_ip_blank",
			server: domain.Server{ServerIP: "10.0.0.5", Metadata: domain.Metadata{"public_ip": "  "}},
			want:   "10.0.0.5",
		},
		{
			name:   "real_ip_when_public_ip_not_a_string",
			server: domain.Server{ServerIP: "10.0.0.5", Metadata: domain.Metadata{"public_ip": true}},
			want:   "10.0.0.5",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, test.server.VisibleServerIP())
		})
	}
}
