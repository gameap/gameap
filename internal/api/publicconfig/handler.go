package publicconfig

import (
	"context"
	"net/http"

	"github.com/gameap/gameap/internal/config"
)

type Responder interface {
	WriteError(ctx context.Context, rw http.ResponseWriter, err error)
	Write(ctx context.Context, rw http.ResponseWriter, result any)
}

type Handler struct {
	config    *config.Config
	responder Responder
}

func NewHandler(cfg *config.Config, responder Responder) *Handler {
	return &Handler{
		config:    cfg,
		responder: responder,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	resp := Response{
		DefaultLanguage: h.config.UI.DefaultLanguage,
	}

	if h.config.Captcha.Provider != "" {
		resp.Captcha = &CaptchaConfig{
			Provider:    h.config.Captcha.Provider,
			SiteKey:     h.config.Captcha.SiteKey,
			InstanceURL: h.config.Captcha.InstanceURL,
		}
	}

	h.responder.Write(ctx, w, resp)
}
