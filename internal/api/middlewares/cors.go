package middlewares

import (
	"fmt"
	"net/http"

	"github.com/gameap/gameap/internal/config"
	"github.com/rs/cors"
)

type CORSMiddleware struct {
	cors *cors.Cors
}

func NewCORSMiddleware(config *config.Config) *CORSMiddleware {
	origin := "http://" + config.HTTPHost

	if config.HTTPPort != 80 && config.HTTPPort != 443 {
		origin = fmt.Sprintf("%s:%d", origin, config.HTTPPort)
	}

	return &CORSMiddleware{
		cors: cors.New(cors.Options{
			AllowedOrigins:   []string{origin},
			AllowCredentials: true,
		}),
	}
}

func (m *CORSMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.cors.Handler(next).ServeHTTP(w, r)
	})
}
