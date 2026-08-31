package middlewares

import (
	"net/http"
	"strconv"
	"strings"
)

// acmeChallengePathPrefix is the well-known path that must remain reachable
// over plain HTTP for ACME HTTP-01 validation. Mirrored from
// internal/acme/http01 to avoid a cyclic import.
const acmeChallengePathPrefix = "/.well-known/acme-challenge/"

// HTTPSRedirectMiddleware sends plain HTTP traffic to HTTPS. canonicalHosts
// are the names the certificate covers: when the list holds at least one
// non-wildcard name, a request carrying any other Host is redirected to the
// first such name instead of to the name the client asked for. Wildcard
// entries (*.example.com) cannot form a valid Location target, so a list of
// only wildcards behaves like an empty one: the client's Host is kept.
func HTTPSRedirectMiddleware(httpsPort uint16, canonicalHosts []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(canonicalHosts))
	for _, h := range canonicalHosts {
		allowed[strings.ToLower(h)] = struct{}{}
	}

	canonicalTarget := ""
	for _, h := range canonicalHosts {
		if !strings.HasPrefix(h, "*") {
			canonicalTarget = h

			break
		}
	}

	// The Host header is client-controlled, so a cache sitting in front of the
	// HTTP port would hand every visitor whatever Location the first
	// requester's header produced.
	redirectHost := func(host string) string {
		if canonicalTarget == "" {
			return host
		}

		if _, ok := allowed[strings.ToLower(host)]; ok {
			return host
		}

		return canonicalTarget
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.TLS == nil && !strings.HasPrefix(r.URL.Path, acmeChallengePathPrefix) {
				host := r.Host
				if idx := strings.LastIndex(host, ":"); idx != -1 {
					host = host[:idx]
				}

				target := "https://" + redirectHost(host)
				if httpsPort != 443 {
					target += ":" + strconv.Itoa(int(httpsPort))
				}
				target += r.URL.RequestURI()

				//nolint:gosec // G710: host is allow-listed above, or the one
				// the client itself asked for when the panel knows no usable
				// canonical name.
				http.Redirect(w, r, target, http.StatusMovedPermanently)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
