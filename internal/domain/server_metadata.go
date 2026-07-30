package domain

import "strings"

// Panel-side settings that live in the generic Server.Metadata bag rather than
// in dedicated columns. Access only through the typed accessors below so the
// key names never leak into handlers.
const (
	metaKeyPublicIP = "public_ip"
)

// PublicIP returns the address an admin published for this server, or an empty
// string when none is set. Only string values count: metadata is free-form, and
// anything else would render as Go syntax instead of an address.
func (s *Server) PublicIP() string {
	if s.Metadata == nil {
		return ""
	}

	raw, ok := s.Metadata[metaKeyPublicIP]
	if !ok {
		return ""
	}

	str, ok := raw.(string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(str)
}

// VisibleServerIP returns the address to display for this server. ServerIP holds
// the address the daemon connects to, which for a node behind NAT is a LAN
// address that must not be published, so PublicIP wins whenever it is set. The
// override is role-independent: every viewer, admins included, sees one address.
func (s *Server) VisibleServerIP() string {
	if publicIP := s.PublicIP(); publicIP != "" {
		return publicIP
	}

	return s.ServerIP
}
