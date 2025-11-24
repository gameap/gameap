package validation

import "testing"

func TestIsValidIPOrHostname(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		// Valid IPv4 addresses
		{name: "valid_IPv4_private", value: "192.168.1.1", want: true},
		{name: "valid_IPv4_public", value: "8.8.8.8", want: true},
		{name: "valid_IPv4_localhost", value: "127.0.0.1", want: true},
		{name: "valid_IPv4_zero", value: "0.0.0.0", want: true},

		// Valid IPv6 addresses
		{name: "valid_IPv6_full", value: "2001:0db8:85a3:0000:0000:8a2e:0370:7334", want: true},
		{name: "valid_IPv6_compressed", value: "2001:db8:85a3::8a2e:370:7334", want: true},
		{name: "valid_IPv6_localhost", value: "::1", want: true},
		{name: "valid_IPv6_unspecified", value: "::", want: true},

		// Valid hostnames
		{name: "valid_hostname_simple", value: "example.com", want: true},
		{name: "valid_hostname_subdomain", value: "www.example.com", want: true},
		{name: "valid_hostname_multiple_subdomains", value: "mail.server.example.com", want: true},
		{name: "valid_hostname_with_hyphen", value: "game-server.example.com", want: true},
		{name: "valid_hostname_short", value: "hldm.org", want: true},
		{name: "valid_hostname_single_word", value: "gameap-daemon", want: true},
		{name: "valid_hostname_number_start", value: "123server.com", want: true},

		// Invalid IP addresses (look like IPs but aren't)
		{name: "invalid_IPv4_out_of_range", value: "192.168.1.999", want: false},
		{name: "invalid_IPv4_too_many_octets", value: "192.168.1.1.1", want: false},
		{name: "invalid_IPv4_too_few_octets", value: "192.168.1", want: false},

		// Invalid hostnames
		{name: "invalid_hostname_special_chars", value: "invalid!!!", want: false},
		{name: "invalid_hostname_underscore", value: "invalid_host", want: false},
		{name: "invalid_hostname_space", value: "invalid host", want: false},
		{name: "invalid_hostname_starts_with_hyphen", value: "-invalid.com", want: false},
		{name: "invalid_hostname_ends_with_hyphen", value: "invalid-.com", want: false},
		{name: "invalid_hostname_empty", value: "", want: false},
		{name: "invalid_hostname_too_long", value: string(make([]byte, 254)), want: false},
		{name: "invalid_hostname_dot_only", value: "...", want: false},
		{name: "invalid_hostname_starts_with_dot", value: ".example.com", want: false},
		{name: "invalid_hostname_ends_with_dot", value: "example.com.", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidIPOrHostname(tt.value); got != tt.want {
				t.Errorf("IsValidIPOrHostname(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
