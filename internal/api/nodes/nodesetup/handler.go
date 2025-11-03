package nodesetup

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	daemonbase "github.com/gameap/gameap/internal/api/daemon/base"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	stringspkg "github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
)

const (
	createTokenLength = 24
	setupTokenTTL     = 300 * time.Second
)

type Handler struct {
	cache     cache.Cache
	responder base.Responder
	panelHost string
}

func NewHandler(
	cache cache.Cache,
	responder base.Responder,
	panelHost string,
) *Handler {
	return &Handler{
		cache:     cache,
		responder: responder,
		panelHost: panelHost,
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

	var err error

	autoSetupToken := os.Getenv("DAEMON_SETUP_TOKEN")
	if autoSetupToken == "" {
		autoSetupToken, err = stringspkg.CryptoRandomString(createTokenLength)
		if err != nil {
			h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate setup token"))

			return
		}
	}

	err = h.cache.Set(
		ctx,
		daemonbase.AutoSetupTokenCacheKey,
		autoSetupToken,
		cache.WithExpiration(setupTokenTTL),
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to store setup token"))

		return
	}

	baseURL := h.detectBaseURL(r)

	response := newSetupResponse(autoSetupToken, baseURL)

	h.responder.Write(ctx, rw, response)
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
