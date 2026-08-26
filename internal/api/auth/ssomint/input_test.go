// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — binding a ticket to a client address
//     only strengthens authentication when the bound value is a real IP. A
//     hostname or a whitespace-padded value would never match the redeeming
//     address (compared verbatim in the exchange handler) and would silently
//     disable the binding, so it must be rejected at mint time.
//   - API3:2023 Broken Object Property Level Authorization — client_ip is an
//     externally supplied property and must be validated, not carried through
//     from arbitrary input.
//
// Reference: https://owasp.org/API-Security/editions/2023/

package ssomint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTicketInput_ValidateClientIP(t *testing.T) {
	t.Parallel()

	const wantInvalid = "client_ip must be a valid IP address"

	tests := []struct {
		name      string
		clientIP  string
		wantError string
	}{
		{name: "empty_is_allowed", clientIP: ""},
		{name: "valid_ipv4", clientIP: "203.0.113.5"},
		{name: "valid_ipv6", clientIP: "2001:db8::1"},
		{name: "ipv6_loopback", clientIP: "::1"},
		{name: "hostname_is_rejected", clientIP: "panel.example.com", wantError: wantInvalid},
		{name: "malformed_octet_is_rejected", clientIP: "999.1.1.1", wantError: wantInvalid},
		{name: "partial_address_is_rejected", clientIP: "203.0.113", wantError: wantInvalid},
		{name: "whitespace_padded_is_rejected", clientIP: " 203.0.113.5 ", wantError: wantInvalid},
		{name: "address_with_port_is_rejected", clientIP: "203.0.113.5:8080", wantError: wantInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			in := &ticketInput{UserID: 1, ClientIP: test.clientIP}

			err := in.Validate()

			if test.wantError == "" {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantError)
		})
	}
}
