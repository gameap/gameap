package enrollment

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatConnectURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		host     string
		port     uint16
		setupKey string
		want     string
	}{
		{
			name:     "standard_format",
			host:     "panel.example.com",
			port:     31718,
			setupKey: "AbCdEfGh1234567890AbCdEfGh123456",
			want:     "grpc://panel.example.com:31718/AbCdEfGh1234567890AbCdEfGh123456",
		},
		{
			name:     "ip_address_host",
			host:     "192.168.1.100",
			port:     9090,
			setupKey: "testkey123",
			want:     "grpc://192.168.1.100:9090/testkey123",
		},
		{
			name:     "custom_port",
			host:     "localhost",
			port:     443,
			setupKey: "key",
			want:     "grpc://localhost:443/key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatConnectURL(tt.host, tt.port, tt.setupKey)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseConnectURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rawURL    string
		want      *ConnectInfo
		wantError string
	}{
		{
			name:   "valid_url",
			rawURL: "grpc://panel.example.com:31718/AbCdEfGh1234567890AbCdEfGh123456",
			want: &ConnectInfo{
				Host:     "panel.example.com",
				Port:     31718,
				SetupKey: "AbCdEfGh1234567890AbCdEfGh123456",
			},
		},
		{
			name:   "ip_address",
			rawURL: "grpc://10.0.0.1:9090/mykey",
			want: &ConnectInfo{
				Host:     "10.0.0.1",
				Port:     9090,
				SetupKey: "mykey",
			},
		},
		{
			name:      "wrong_scheme",
			rawURL:    "http://panel.example.com:31718/key",
			wantError: "invalid connect URL",
		},
		{
			name:      "missing_host",
			rawURL:    "grpc://:31718/key",
			wantError: "host is required",
		},
		{
			name:      "missing_port",
			rawURL:    "grpc://panel.example.com/key",
			wantError: "port is required",
		},
		{
			name:      "missing_key",
			rawURL:    "grpc://panel.example.com:31718/",
			wantError: "setup key is required",
		},
		{
			name:      "missing_key_no_slash",
			rawURL:    "grpc://panel.example.com:31718",
			wantError: "setup key is required",
		},
		{
			name:      "invalid_port",
			rawURL:    "grpc://panel.example.com:99999/key",
			wantError: "invalid port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseConnectURL(tt.rawURL)
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatParseRoundTrip(t *testing.T) {
	t.Parallel()

	host := "panel.example.com"
	port := uint16(31718)
	key := "AbCdEfGh1234567890AbCdEfGh123456"

	formatted := FormatConnectURL(host, port, key)
	parsed, err := ParseConnectURL(formatted)
	require.NoError(t, err)

	assert.Equal(t, host, parsed.Host)
	assert.Equal(t, port, parsed.Port)
	assert.Equal(t, key, parsed.SetupKey)
}
