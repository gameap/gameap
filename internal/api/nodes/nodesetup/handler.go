package nodesetup

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/enrollment"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	responder       base.Responder
	panelHost       string
	enrollmentSvc   *enrollment.Service
	grpcPort        uint16
	grpcExtHost     string
	grpcExtPort     uint16
	certHostCovered func(host string) bool
}

func NewHandler(
	responder base.Responder,
	panelHost string,
	enrollmentSvc *enrollment.Service,
	grpcPort uint16,
	grpcExtHost string,
	grpcExtPort uint16,
	certHostCovered func(host string) bool,
) *Handler {
	return &Handler{
		responder:       responder,
		panelHost:       panelHost,
		enrollmentSvc:   enrollmentSvc,
		grpcPort:        grpcPort,
		grpcExtHost:     grpcExtHost,
		grpcExtPort:     grpcExtPort,
		certHostCovered: certHostCovered,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := auth.SessionFromContext(ctx)
	if !session.IsAuthenticated() {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("user not authenticated"),
			http.StatusUnauthorized,
		))

		return
	}

	setupKey, err := h.enrollmentSvc.SetupKeyManager().Generate(ctx)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate setup key"))

		return
	}

	grpcHost := h.resolveGRPCHost(r)
	grpcPort := h.grpcPort
	if h.grpcExtPort > 0 {
		grpcPort = h.grpcExtPort
	}

	connectURL := enrollment.FormatConnectURL(grpcHost, grpcPort, setupKey)
	baseURL := h.detectBaseURL(r)
	setupLink := baseURL + "/nodes/setup/" + setupKey
	linuxCmd := "bash <(curl -s '" + setupLink + "')"
	windowsCmd := "gameapctl daemon install --connect=" + connectURL

	var warnings []string
	if h.certHostCovered != nil && !h.certHostCovered(grpcHost) {
		slog.WarnContext(ctx, "resolved gRPC connect host is not covered by the panel gRPC TLS certificate; "+
			"daemons will fail TLS verification when connecting via this address. "+
			"Set GRPC_EXTERNAL_HOST to a stable address and restart the panel "+
			"(its gRPC certificate is regenerated automatically)",
			slog.String("grpc_host", grpcHost),
		)
		warnings = append(warnings,
			"gRPC connect host \""+grpcHost+"\" is not covered by the panel gRPC TLS certificate. "+
				"Daemons will fail TLS verification when connecting via this address. "+
				"Set GRPC_EXTERNAL_HOST in the panel configuration and restart the panel "+
				"to regenerate the certificate.")
	}

	h.responder.Write(ctx, rw, setupResponse{
		Link:        setupLink,
		Token:       setupKey,
		Host:        baseURL,
		GRPCEnabled: true,
		ConnectURL:  connectURL,
		LinuxCmd:    linuxCmd,
		WindowsCmd:  windowsCmd,
		SetupLink:   setupLink,
		Warnings:    warnings,
	})
}

func (h *Handler) resolveGRPCHost(r *http.Request) string {
	if h.grpcExtHost != "" {
		return h.grpcExtHost
	}

	host := h.panelHost
	if host == "" {
		host = r.Header.Get("X-Forwarded-Host")
		if host == "" {
			host = r.Host
		}
	}

	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")

	if idx := strings.IndexByte(host, ':'); idx != -1 {
		host = host[:idx]
	}

	return host
}

func (h *Handler) detectBaseURL(r *http.Request) string {
	host := h.panelHost

	if host == "" {
		host = r.Header.Get("X-Forwarded-Host")
		if host == "" {
			host = r.Host
		}
	}

	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")

	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}

	b := strings.Builder{}
	b.Grow(len(scheme) + len(host) + 3)
	b.WriteString(scheme)
	b.WriteString("://")
	b.WriteString(host)

	return b.String()
}
