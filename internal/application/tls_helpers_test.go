package application

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveGRPCCertSANs(t *testing.T) {
	tests := []struct {
		name         string
		httpHost     string
		httpBindIP   string
		externalHost string
		stubAuto     []sanSource
		wantIPs      []string
		wantDNS      []string
		wantSources  map[string]string
	}{
		{
			name:         "all_empty_inputs_returns_only_fallback",
			httpHost:     "",
			httpBindIP:   "",
			externalHost: "",
			stubAuto:     nil,
			wantIPs:      []string{"127.0.0.1"},
			wantDNS:      []string{"localhost"},
			wantSources: map[string]string{
				"ip:127.0.0.1":  "fallback",
				"dns:localhost": "fallback",
			},
		},
		{
			name:         "bind_all_with_auto_detected_interface_ips",
			httpHost:     "0.0.0.0",
			httpBindIP:   "",
			externalHost: "",
			stubAuto: []sanSource{
				{ip: net.ParseIP("192.168.8.174"), from: "auto:eth0"},
				{ip: net.ParseIP("10.0.0.5"), from: "auto:eth1"},
			},
			wantIPs: []string{"192.168.8.174", "10.0.0.5", "127.0.0.1"},
			wantDNS: []string{"localhost"},
			wantSources: map[string]string{
				"ip:192.168.8.174": "auto:eth0",
				"ip:10.0.0.5":      "auto:eth1",
				"ip:127.0.0.1":     "fallback",
				"dns:localhost":    "fallback",
			},
		},
		{
			name:         "external_host_as_dns_name",
			httpHost:     "0.0.0.0",
			httpBindIP:   "",
			externalHost: "api.example.com",
			stubAuto:     nil,
			wantIPs:      []string{"127.0.0.1"},
			wantDNS:      []string{"api.example.com", "localhost"},
			wantSources: map[string]string{
				"dns:api.example.com": "config:GRPC_EXTERNAL_HOST",
				"ip:127.0.0.1":        "fallback",
				"dns:localhost":       "fallback",
			},
		},
		{
			name:         "config_bind_ip_dedupes_with_auto_detected",
			httpHost:     "0.0.0.0",
			httpBindIP:   "10.0.0.1",
			externalHost: "",
			stubAuto: []sanSource{
				{ip: net.ParseIP("10.0.0.1"), from: "auto:eth0"},
				{ip: net.ParseIP("192.168.1.5"), from: "auto:eth1"},
			},
			wantIPs: []string{"10.0.0.1", "192.168.1.5", "127.0.0.1"},
			wantDNS: []string{"localhost"},
			wantSources: map[string]string{
				"ip:10.0.0.1":    "config:HTTP_BIND_IP",
				"ip:192.168.1.5": "auto:eth1",
				"ip:127.0.0.1":   "fallback",
				"dns:localhost":  "fallback",
			},
		},
		{
			name:         "specific_http_host_appends_auto_detect",
			httpHost:     "192.168.0.1",
			httpBindIP:   "",
			externalHost: "",
			stubAuto: []sanSource{
				{ip: net.ParseIP("10.0.0.99"), from: "auto:eth9"},
			},
			wantIPs: []string{"192.168.0.1", "10.0.0.99", "127.0.0.1"},
			wantDNS: []string{"localhost"},
			wantSources: map[string]string{
				"ip:192.168.0.1": "config:HTTP_HOST",
				"ip:10.0.0.99":   "auto:eth9",
				"ip:127.0.0.1":   "fallback",
				"dns:localhost":  "fallback",
			},
		},
		{
			name:         "nat_local_http_host_keeps_config_source_and_appends_other_interfaces",
			httpHost:     "10.73.43.20",
			httpBindIP:   "",
			externalHost: "",
			stubAuto: []sanSource{
				{ip: net.ParseIP("10.73.43.20"), from: "auto:eth0"},
				{ip: net.ParseIP("192.168.1.5"), from: "auto:eth1"},
			},
			wantIPs: []string{"10.73.43.20", "192.168.1.5", "127.0.0.1"},
			wantDNS: []string{"localhost"},
			wantSources: map[string]string{
				"ip:10.73.43.20": "config:HTTP_HOST",
				"ip:192.168.1.5": "auto:eth1",
				"ip:127.0.0.1":   "fallback",
				"dns:localhost":  "fallback",
			},
		},
		{
			name:         "all_three_config_sources_combined",
			httpHost:     "0.0.0.0",
			httpBindIP:   "10.0.0.1",
			externalHost: "panel.example.com",
			stubAuto:     nil,
			wantIPs:      []string{"10.0.0.1", "127.0.0.1"},
			wantDNS:      []string{"panel.example.com", "localhost"},
			wantSources: map[string]string{
				"ip:10.0.0.1":           "config:HTTP_BIND_IP",
				"dns:panel.example.com": "config:GRPC_EXTERNAL_HOST",
				"ip:127.0.0.1":          "fallback",
				"dns:localhost":         "fallback",
			},
		},
		{
			name:         "loopback_explicitly_in_config_keeps_config_source",
			httpHost:     "127.0.0.1",
			httpBindIP:   "",
			externalHost: "",
			stubAuto:     nil,
			wantIPs:      []string{"127.0.0.1"},
			wantDNS:      []string{"localhost"},
			wantSources: map[string]string{
				"ip:127.0.0.1":  "config:HTTP_HOST",
				"dns:localhost": "fallback",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalDetect := detectInterfaceSANs
			t.Cleanup(func() {
				detectInterfaceSANs = originalDetect
			})

			stub := tt.stubAuto
			detectInterfaceSANs = func() []sanSource { return stub }

			got := resolveGRPCCertSANs(tt.httpHost, tt.httpBindIP, tt.externalHost)

			ips, dnsNames := splitSANs(got)
			assert.Equal(t, tt.wantIPs, ipAddressesToStrings(ips), "ip addresses")
			assert.Equal(t, tt.wantDNS, dnsNames, "dns names")

			require.Len(t, got, len(tt.wantSources), "total entry count")
			for _, s := range got {
				var key string
				switch {
				case s.ip != nil:
					key = "ip:" + s.ip.String()
				case s.dns != "":
					key = "dns:" + s.dns
				}
				wantFrom, ok := tt.wantSources[key]
				assert.True(t, ok, "unexpected entry %q", key)
				assert.Equal(t, wantFrom, s.from, "source for %q", key)
			}
		})
	}
}

func TestRealDetectInterfaceSANs_excludes_loopback_and_link_local(t *testing.T) {
	got := realDetectInterfaceSANs()

	for _, s := range got {
		require.NotNil(t, s.ip, "auto entries must have an IP")
		assert.False(t, s.ip.IsLoopback(), "loopback %s must not be auto-detected", s.ip)
		assert.False(t, s.ip.IsLinkLocalUnicast(), "link-local unicast %s must not be auto-detected", s.ip)
		assert.False(t, s.ip.IsLinkLocalMulticast(), "link-local multicast %s must not be auto-detected", s.ip)
		assert.False(t, s.ip.IsUnspecified(), "unspecified %s must not be auto-detected", s.ip)
		assert.Contains(t, s.from, "auto:", "source label must start with auto:")
	}
}

func TestFormatSANSourcesForLog(t *testing.T) {
	sources := []sanSource{
		{ip: net.ParseIP("192.168.8.174"), from: "auto:eth0"},
		{dns: "api.example.com", from: "config:GRPC_EXTERNAL_HOST"},
		{ip: net.ParseIP("127.0.0.1"), from: "fallback"},
		{dns: "localhost", from: "fallback"},
	}

	ipsLog, dnsLog := formatSANSourcesForLog(sources)

	assert.Equal(t, []string{
		"192.168.8.174 (auto:eth0)",
		"127.0.0.1 (fallback)",
	}, ipsLog)
	assert.Equal(t, []string{
		"api.example.com (config:GRPC_EXTERNAL_HOST)",
		"localhost (fallback)",
	}, dnsLog)
}
