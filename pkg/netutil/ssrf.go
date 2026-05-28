package netutil

import (
	"net/netip"
)

// Block reasons. Stable strings — emitted in audit events and error
// messages, change them only with a coordinated update of the audit
// catalogue.
const (
	BlockReasonLoopback      = "loopback"
	BlockReasonUnspecified   = "unspecified"
	BlockReasonLinkLocal     = "link_local"
	BlockReasonPrivate       = "private"
	BlockReasonMulticast     = "multicast"
	BlockReasonCloudMetadata = "cloud_metadata"
	BlockReasonCGNAT         = "cgnat"
	BlockReasonReservedV4    = "reserved_v4"
)

// IsBlockedIP reports whether an outbound dial to this IP must be refused
// for SSRF safety. It is the conservative blocklist used by the plugin
// HTTP host library: every address that could route to a panel-local
// service, a hypervisor metadata endpoint, an internal corporate network
// or a multicast scope is blocked.
//
// Cloud metadata addresses (169.254.169.254 etc.) are blocked here AND
// reported separately by IsCloudMetadataIP so an allow-list cannot
// re-enable them (see BlockReason).
//
// The function works on netip.Addr (Go's modern, value-typed IP) so
// callers can pass results from net.Resolver.LookupNetIP directly
// without conversions.
func IsBlockedIP(ip netip.Addr) bool {
	return BlockReason(ip) != ""
}

// IsCloudMetadataIP returns true for the well-known cloud-provider
// metadata endpoints. These IPs are link-local (169.254.0.0/16 on IPv4
// or fd00::/8 ULA on IPv6) so IsBlockedIP already covers them, but we
// keep the dedicated check so operator allow-lists for "private IPs"
// cannot accidentally re-enable IMDS access.
func IsCloudMetadataIP(ip netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}

	// AWS / GCP / Azure / DigitalOcean / Oracle IMDS v4.
	if ip.Is4() && ip == netip.MustParseAddr("169.254.169.254") {
		return true
	}

	// Alibaba Cloud IMDS.
	if ip.Is4() && ip == netip.MustParseAddr("100.100.100.200") {
		return true
	}

	// AWS IMDS over IPv6.
	if ip.Is6() && ip == netip.MustParseAddr("fd00:ec2::254") {
		return true
	}

	return false
}

// BlockReason returns a stable, audit-friendly identifier for why an IP is
// blocked, or "" if the IP is acceptable. Categories are checked in a
// fixed order so an IP that matches multiple categories (e.g. a cloud
// metadata address which is also link-local) reports the most-specific
// reason first.
//
// Order:
//
//  1. cloud_metadata (most specific — must never be allow-list-able)
//  2. loopback
//  3. unspecified (0.0.0.0, ::)
//  4. link_local (RFC 3927 / RFC 4291)
//  5. multicast
//  6. private (RFC 1918 / RFC 4193 ULA)
//  7. cgnat (RFC 6598 100.64.0.0/10)
//  8. reserved_v4 (0.0.0.0/8 source-only, 240.0.0.0/4 future, broadcast)
func BlockReason(ip netip.Addr) string {
	if !ip.IsValid() {
		return BlockReasonReservedV4
	}

	if IsCloudMetadataIP(ip) {
		return BlockReasonCloudMetadata
	}

	if ip.IsLoopback() {
		return BlockReasonLoopback
	}

	if ip.IsUnspecified() {
		return BlockReasonUnspecified
	}

	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return BlockReasonLinkLocal
	}

	if ip.IsMulticast() {
		return BlockReasonMulticast
	}

	if ip.IsPrivate() {
		return BlockReasonPrivate
	}

	if isCGNAT(ip) {
		return BlockReasonCGNAT
	}

	if isReservedV4(ip) {
		return BlockReasonReservedV4
	}

	return ""
}

// isCGNAT covers RFC 6598 100.64.0.0/10. Go's netip.Addr.IsPrivate does
// NOT include this range (RFC 1918 only), but a panel deployed inside a
// carrier-grade NAT segment can still reach neighbour customer subnets,
// so we block it.
func isCGNAT(ip netip.Addr) bool {
	if !ip.Is4() {
		return false
	}

	a4 := ip.As4()
	// 100.64.0.0/10 -> first octet 100, second 64..127.
	return a4[0] == 100 && a4[1] >= 64 && a4[1] <= 127
}

// isReservedV4 covers the IPv4 reserved ranges that don't fall under
// loopback/unspecified/link-local/multicast/private but should still
// never appear in an outbound request: the 0.0.0.0/8 source-only range
// (RFC 6890) and the 240.0.0.0/4 future-use range (RFC 1112) including
// the all-ones broadcast 255.255.255.255.
func isReservedV4(ip netip.Addr) bool {
	if !ip.Is4() {
		return false
	}

	a4 := ip.As4()

	// 0.0.0.0/8 (the 0.0.0.0 = unspecified case is already handled above,
	// but 0.x.x.x for any x is still "this network" and not routable).
	if a4[0] == 0 {
		return true
	}

	// 240.0.0.0/4 (future use) — covers 255.255.255.255 too.
	if a4[0] >= 240 {
		return true
	}

	return false
}
