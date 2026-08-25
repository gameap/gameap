package enrollsetup

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/enrollment"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

type Handler struct {
	enrollmentSvc   *enrollment.Service
	responder       base.Responder
	grpcExtHost     string
	grpcPort        uint16
	grpcExtPort     uint16
	panelHost       string
	certHostCovered func(host string) bool
}

func NewHandler(
	enrollmentSvc *enrollment.Service,
	responder base.Responder,
	panelHost string,
	grpcExtHost string,
	grpcPort uint16,
	grpcExtPort uint16,
	certHostCovered func(host string) bool,
) *Handler {
	return &Handler{
		enrollmentSvc:   enrollmentSvc,
		responder:       responder,
		panelHost:       panelHost,
		grpcExtHost:     grpcExtHost,
		grpcPort:        grpcPort,
		grpcExtPort:     grpcExtPort,
		certHostCovered: certHostCovered,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.enrollmentSvc == nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("enrollment is not available, gRPC is disabled"),
			http.StatusServiceUnavailable,
		))

		return
	}

	key, err := api.NewInputReader(r).ReadString("key")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid key"),
			http.StatusBadRequest,
		))

		return
	}

	if err := h.enrollmentSvc.ValidateSetupKey(ctx, key); err != nil {
		if errors.Is(err, enrollment.ErrInvalidSetupKey) || errors.Is(err, enrollment.ErrSetupKeyNotConfigured) {
			h.responder.WriteError(ctx, rw, api.WrapHTTPError(
				errors.WithMessage(err, "invalid setup key"),
				http.StatusForbidden,
			))

			return
		}

		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to validate setup key"))

		return
	}

	grpcHost := h.resolveGRPCHost(r)
	grpcPort := h.grpcPort
	if h.grpcExtPort > 0 {
		grpcPort = h.grpcExtPort
	}

	if h.certHostCovered != nil && !h.certHostCovered(grpcHost) {
		slog.WarnContext(ctx, "resolved gRPC connect host is not covered by the panel gRPC TLS certificate; "+
			"daemons will fail TLS verification when connecting via this address. "+
			"Set GRPC_EXTERNAL_HOST to a stable address and restart the panel "+
			"(its gRPC certificate is regenerated automatically)",
			slog.String("grpc_host", grpcHost),
		)
	}

	connectURL := enrollment.FormatConnectURL(grpcHost, grpcPort, key)

	queryReader := api.NewQueryReader(r)
	config, _ := queryReader.ReadString("config")
	github, _ := queryReader.ReadString("github")
	branch, _ := queryReader.ReadString("branch")

	script := enrollment.BuildSetupScript(connectURL, enrollment.SetupScriptOptions{
		Config: config,
		GitHub: github == "true",
		Branch: branch,
	})

	rw.Header().Set("Content-Type", "text/plain")
	// The script is served as text/plain, never as HTML, and every value
	// interpolated into it is shell-escaped by BuildSetupScript.
	_, _ = rw.Write([]byte(script)) //nolint:gosec // not HTML; interpolated values are shell-escaped
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
