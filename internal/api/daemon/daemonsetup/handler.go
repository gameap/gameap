package daemonsetup

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	daemonbase "github.com/gameap/gameap/internal/api/daemon/base"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/pkg/api"
	stringspkg "github.com/gameap/gameap/pkg/strings"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

const (
	createTokenLength = 24
	createTokenTTL    = 3600 * time.Second
)

var (
	ErrInvalidSetupToken = errors.New("invalid setup token")
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

	vars := mux.Vars(r)
	token := vars["token"]

	err := h.verifySetupToken(ctx, token)
	if err != nil && errors.Is(err, ErrInvalidSetupToken) {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid setup token"),
			http.StatusForbidden,
		))

		return
	}
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to verify setup token"))

		return
	}

	err = h.cache.Delete(ctx, daemonbase.AutoSetupTokenCacheKey)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to clear setup token"))

		return
	}

	createToken, err := h.generateCreateToken(ctx)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate create token"))

		return
	}

	baseURL := h.detectBaseURL(r)

	script := h.buildSetupScript(createToken, baseURL)

	rw.Header().Set("Content-Type", "text/plain")
	_, _ = rw.Write([]byte(script))
}

func (h *Handler) verifySetupToken(ctx context.Context, token string) error {
	autoSetupToken := os.Getenv("DAEMON_SETUP_TOKEN")

	//nolint:nestif
	if autoSetupToken == "" {
		val, err := h.cache.Get(ctx, daemonbase.AutoSetupTokenCacheKey)
		if err != nil {
			if errors.Is(err, cache.ErrNotFound) {
				return ErrInvalidSetupToken
			}

			return errors.WithMessage(err, "failed to get setup token from cache")
		}

		if val == nil {
			return ErrInvalidSetupToken
		}

		var ok bool
		autoSetupToken, ok = val.(string)
		if !ok {
			return errors.New("invalid setup token type in cache")
		}
	}

	if token != autoSetupToken {
		return ErrInvalidSetupToken
	}

	return nil
}

func (h *Handler) generateCreateToken(ctx context.Context) (string, error) {
	token, err := stringspkg.CryptoRandomString(createTokenLength)
	if err != nil {
		return "", errors.WithMessage(err, "failed to generate random token")
	}

	err = h.cache.Set(
		ctx,
		daemonbase.AutoCreateTokenCacheKey,
		token,
		cache.WithExpiration(createTokenTTL),
	)
	if err != nil {
		return "", errors.WithMessage(err, "failed to store create token")
	}

	return token, nil
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

func (h *Handler) buildSetupScript(createToken, panelHost string) string {
	sb := strings.Builder{}
	sb.Grow(256)

	sb.WriteString("export createToken=")
	sb.WriteString(createToken)
	sb.WriteString(";\n")
	sb.WriteString("export panelHost=")
	sb.WriteString(panelHost)
	sb.WriteString(";\n")
	sb.WriteString(
		"curl -sL https://raw.githubusercontent.com/gameap/auto-install-scripts/master/install-gdaemon.sh | bash --",
	)

	return sb.String()
}
