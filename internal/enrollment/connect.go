package enrollment

import (
	"net"
	"strings"

	"github.com/pkg/errors"
)

// ErrConnectHostUnresolved is returned when neither GRPC_EXTERNAL_HOST nor a
// caller-supplied fallback names the address daemons should dial. Guessing
// would produce a connect URL the machine cannot use, so callers fail instead.
var ErrConnectHostUnresolved = errors.New(
	"gRPC connect host is not configured: set GRPC_EXTERNAL_HOST or pass a connect host",
)

// ConnectTarget is the address a daemon is told to dial, plus any operator
// warnings collected while resolving it.
type ConnectTarget struct {
	Host     string
	Port     uint16
	Warnings []string
}

// ConnectResolver turns the panel's gRPC configuration into the address that
// goes into a connect URL. HTTP handlers pass the request-derived host as the
// fallback; host libraries have no request and pass whatever the plugin named.
type ConnectResolver struct {
	externalHost    string
	port            uint16
	externalPort    uint16
	certHostCovered func(host string) bool
}

func NewConnectResolver(
	externalHost string,
	port, externalPort uint16,
	certHostCovered func(host string) bool,
) *ConnectResolver {
	return &ConnectResolver{
		externalHost:    externalHost,
		port:            port,
		externalPort:    externalPort,
		certHostCovered: certHostCovered,
	}
}

// Resolve picks the connect host and port. GRPC_EXTERNAL_HOST always wins;
// the fallback is only consulted when it is unset.
func (r *ConnectResolver) Resolve(fallbackHost string) (ConnectTarget, error) {
	host := r.externalHost
	if host == "" {
		host = NormalizeHost(fallbackHost)
	}

	if host == "" {
		return ConnectTarget{}, ErrConnectHostUnresolved
	}

	port := r.port
	if r.externalPort > 0 {
		port = r.externalPort
	}

	target := ConnectTarget{Host: host, Port: port}

	if r.certHostCovered != nil && !r.certHostCovered(host) {
		target.Warnings = append(target.Warnings, CertHostWarning(host))
	}

	return target, nil
}

// NormalizeHost strips a scheme and a port from a host value, so a raw Host
// header or a plugin-supplied "https://panel.example.com:8080" resolves to the
// bare hostname. IPv6 survives in every shape it arrives in: cutting at the
// first colon would turn "[2001:db8::1]:31718" into "[2001".
func NormalizeHost(raw string) string {
	host := strings.TrimSpace(raw)
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")

	if host == "" {
		return ""
	}

	// "panel.example.com:8080" and "[2001:db8::1]:31718".
	if hostname, _, err := net.SplitHostPort(host); err == nil {
		return hostname
	}

	// "[2001:db8::1]" — bracketed, but no port for SplitHostPort to find.
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		return host[1 : len(host)-1]
	}

	// A name without a port, or a bare IPv6 literal that SplitHostPort rejects
	// as having too many colons.
	return host
}

// CertHostWarning is the operator-facing text for a connect host the panel's
// gRPC certificate does not cover; daemons would fail TLS verification.
func CertHostWarning(host string) string {
	return "gRPC connect host \"" + host + "\" is not covered by the panel gRPC TLS certificate. " +
		"Daemons will fail TLS verification when connecting via this address. " +
		"Set GRPC_EXTERNAL_HOST in the panel configuration and restart the panel " +
		"to regenerate the certificate."
}
