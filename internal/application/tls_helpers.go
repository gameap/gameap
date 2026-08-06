package application

import (
	"crypto/tls"
	"log/slog"
	"net"
)

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS1.0"
	case tls.VersionTLS11:
		return "TLS1.1"
	case tls.VersionTLS12:
		return "TLS1.2"
	case tls.VersionTLS13:
		return "TLS1.3"
	default:
		return "unknown"
	}
}

func ipAddressesToStrings(ips []net.IP) []string {
	if len(ips) == 0 {
		return nil
	}

	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.String())
	}

	return out
}

type sanSource struct {
	ip   net.IP
	dns  string
	from string
}

// detectInterfaceSANs returns SAN entries for non-loopback, up network interfaces.
// Replaceable in tests for deterministic behavior.
var detectInterfaceSANs = realDetectInterfaceSANs

func resolveGRPCCertSANs(httpHost, httpBindIP, externalHost string) []sanSource {
	var sources []sanSource
	seen := make(map[string]struct{})

	addEntry := func(s sanSource) {
		var key string
		switch {
		case s.ip != nil:
			key = "ip:" + s.ip.String()
		case s.dns != "":
			key = "dns:" + s.dns
		default:
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		sources = append(sources, s)
	}

	configured := []struct {
		value string
		from  string
	}{
		{httpHost, "config:HTTP_HOST"},
		{httpBindIP, "config:HTTP_BIND_IP"},
		{externalHost, "config:GRPC_EXTERNAL_HOST"},
	}

	for _, c := range configured {
		if c.value == "" || c.value == "0.0.0.0" {
			continue
		}
		if ip := net.ParseIP(c.value); ip != nil {
			addEntry(sanSource{ip: ip, from: c.from})
		} else {
			addEntry(sanSource{dns: c.value, from: c.from})
		}
	}

	// Interface IPs are always included so that a daemon on the same machine can
	// reach the panel by a local address even when HTTP_HOST points elsewhere
	// (e.g. a public address of a NAT in front of this machine).
	for _, s := range detectInterfaceSANs() {
		addEntry(s)
	}

	addEntry(sanSource{ip: net.IPv4(127, 0, 0, 1), from: "fallback"})
	addEntry(sanSource{dns: "localhost", from: "fallback"})

	return sources
}

func realDetectInterfaceSANs() []sanSource {
	interfaces, err := net.Interfaces()
	if err != nil {
		slog.Warn("failed to enumerate network interfaces for cert SANs",
			slog.String("error", err.Error()),
		)

		return nil
	}

	var sources []sanSource
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			slog.Warn("failed to read addresses for interface",
				slog.String("interface", iface.Name),
				slog.String("error", err.Error()),
			)

			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil ||
				ip.IsLoopback() ||
				ip.IsUnspecified() ||
				ip.IsLinkLocalUnicast() ||
				ip.IsLinkLocalMulticast() {
				continue
			}

			sources = append(sources, sanSource{
				ip:   ip,
				from: "auto:" + iface.Name,
			})
		}
	}

	return sources
}

func splitSANs(sources []sanSource) ([]net.IP, []string) {
	var ips []net.IP
	var dnsNames []string
	for _, s := range sources {
		switch {
		case s.ip != nil:
			ips = append(ips, s.ip)
		case s.dns != "":
			dnsNames = append(dnsNames, s.dns)
		}
	}

	return ips, dnsNames
}

func formatSANSourcesForLog(sources []sanSource) (ipsLog, dnsNamesLog []string) {
	for _, s := range sources {
		switch {
		case s.ip != nil:
			ipsLog = append(ipsLog, s.ip.String()+" ("+s.from+")")
		case s.dns != "":
			dnsNamesLog = append(dnsNamesLog, s.dns+" ("+s.from+")")
		}
	}

	return ipsLog, dnsNamesLog
}
