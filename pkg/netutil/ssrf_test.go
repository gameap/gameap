// API Security Tests for OWASP API Security Top 10:2023.
// Category: API7:2023 — Server-Side Request Forgery.
//
// Pins the SSRF blocklist used by the plugin HTTP host library. A
// regression here would re-open the loopback / RFC1918 / cloud-metadata
// pivot from a compromised plugin into the panel's host network.
//
// Reference: https://owasp.org/API-Security/editions/2023/en/0xa7-server-side-request-forgery/
package netutil_test

import (
	"net/netip"
	"testing"

	"github.com/gameap/gameap/pkg/netutil"
	"github.com/stretchr/testify/assert"
)

func TestIsBlockedIP_BlocksLoopback(t *testing.T) {
	cases := []string{
		"127.0.0.1",
		"127.255.255.254",
		"::1",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			ip := netip.MustParseAddr(raw)
			assert.True(t, netutil.IsBlockedIP(ip))
			assert.Equal(t, netutil.BlockReasonLoopback, netutil.BlockReason(ip))
		})
	}
}

func TestIsBlockedIP_BlocksPrivateRFC1918(t *testing.T) {
	cases := []string{
		"10.0.0.1",
		"10.255.255.254",
		"172.16.0.1",
		"172.31.255.254",
		"192.168.0.1",
		"192.168.255.254",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			ip := netip.MustParseAddr(raw)
			assert.True(t, netutil.IsBlockedIP(ip))
			assert.Equal(t, netutil.BlockReasonPrivate, netutil.BlockReason(ip))
		})
	}
}

func TestIsBlockedIP_BlocksIPv6ULA(t *testing.T) {
	cases := []string{
		"fc00::1",
		"fd00::1",
		"fdff:ffff:ffff:ffff::1",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			ip := netip.MustParseAddr(raw)
			assert.True(t, netutil.IsBlockedIP(ip))
			assert.Equal(t, netutil.BlockReasonPrivate, netutil.BlockReason(ip))
		})
	}
}

func TestIsBlockedIP_BlocksLinkLocal(t *testing.T) {
	cases := []string{
		"169.254.1.1", // not the metadata IP
		"169.254.255.254",
		"fe80::1",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			ip := netip.MustParseAddr(raw)
			assert.True(t, netutil.IsBlockedIP(ip))
			assert.Equal(t, netutil.BlockReasonLinkLocal, netutil.BlockReason(ip))
		})
	}
}

func TestIsBlockedIP_BlocksCloudMetadata(t *testing.T) {
	cases := []string{
		"169.254.169.254",
		"fd00:ec2::254",
		"100.100.100.200",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			ip := netip.MustParseAddr(raw)
			assert.True(t, netutil.IsBlockedIP(ip))
			assert.True(t, netutil.IsCloudMetadataIP(ip))
			assert.Equal(t, netutil.BlockReasonCloudMetadata, netutil.BlockReason(ip),
				"cloud-metadata reason must be reported BEFORE the broader link-local reason")
		})
	}
}

func TestIsBlockedIP_BlocksUnspecified(t *testing.T) {
	cases := []string{"0.0.0.0", "::"}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			ip := netip.MustParseAddr(raw)
			assert.True(t, netutil.IsBlockedIP(ip))
			// 0.0.0.0 hits both "unspecified" and "reserved_v4 (0.0.0.0/8)";
			// our ordering reports unspecified first because it's the
			// more specific operator-visible signal.
			assert.Equal(t, netutil.BlockReasonUnspecified, netutil.BlockReason(ip))
		})
	}
}

func TestIsBlockedIP_BlocksMulticast(t *testing.T) {
	cases := []string{
		"224.0.0.1",
		"239.255.255.250", // SSDP
		"ff02::1",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			ip := netip.MustParseAddr(raw)
			assert.True(t, netutil.IsBlockedIP(ip))
			reason := netutil.BlockReason(ip)
			// IPv6 multicast (ff02::) is also link-local-scoped, which
			// matches earlier in the order; either reason is acceptable.
			assert.Contains(t, []string{netutil.BlockReasonMulticast, netutil.BlockReasonLinkLocal}, reason,
				"got %q", reason)
		})
	}
}

func TestIsBlockedIP_BlocksCGNAT(t *testing.T) {
	cases := []string{
		"100.64.0.1",
		"100.127.255.254",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			ip := netip.MustParseAddr(raw)
			assert.True(t, netutil.IsBlockedIP(ip))
			assert.Equal(t, netutil.BlockReasonCGNAT, netutil.BlockReason(ip))
		})
	}
}

func TestIsBlockedIP_BlocksReservedV4(t *testing.T) {
	cases := []string{
		"0.1.2.3",   // 0.0.0.0/8 — "this network"
		"240.0.0.1", // 240.0.0.0/4 — future use
		"255.255.255.255",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			ip := netip.MustParseAddr(raw)
			assert.True(t, netutil.IsBlockedIP(ip))
			assert.Equal(t, netutil.BlockReasonReservedV4, netutil.BlockReason(ip))
		})
	}
}

func TestIsBlockedIP_AllowsPublicAddresses(t *testing.T) {
	cases := []string{
		"8.8.8.8",
		"1.1.1.1",
		"99.99.99.99",
		"203.0.113.42", // TEST-NET-3 — not private, technically routable
		"2001:db8::1",  // documentation prefix — not private/ULA/link-local
		"2606:4700:4700::1111",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			ip := netip.MustParseAddr(raw)
			assert.False(t, netutil.IsBlockedIP(ip), "public IP must be allowed")
			assert.Empty(t, netutil.BlockReason(ip))
		})
	}
}

func TestIsCloudMetadataIP_RejectsLookalikes(t *testing.T) {
	// IPs that are link-local but NOT the metadata endpoint must NOT
	// match IsCloudMetadataIP — otherwise an allow-list bypass that
	// trusts "not metadata" would accidentally allow IMDS access.
	cases := []string{
		"169.254.169.253",
		"169.254.169.255",
		"169.254.1.1",
		"100.100.100.199",
		"100.100.100.201",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			ip := netip.MustParseAddr(raw)
			assert.False(t, netutil.IsCloudMetadataIP(ip))
		})
	}
}

func TestBlockReason_InvalidAddrIsRejected(t *testing.T) {
	// netip.Addr zero value (Addr{}) is !IsValid() — must be rejected.
	var zero netip.Addr
	assert.True(t, netutil.IsBlockedIP(zero))
	assert.Equal(t, netutil.BlockReasonReservedV4, netutil.BlockReason(zero))
}
