package middlewares

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

type RecoveryMiddleware struct {
	responder base.Responder
}

func NewRecoveryMiddleware(responder base.Responder) *RecoveryMiddleware {
	return &RecoveryMiddleware{
		responder: responder,
	}
}

func (m *RecoveryMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// net/http's own recovery treats this sentinel as "drop the connection without
				// logging" — the way a streaming handler signals failure after headers were sent.
				// Swallowing it here would let the response end as if it were complete.
				if e, ok := err.(error); ok && errors.Is(e, http.ErrAbortHandler) {
					panic(err)
				}

				// Capture stack trace
				stack := debug.Stack()

				// Log the panic with structured logging
				slog.LogAttrs(
					r.Context(),
					slog.LevelError,
					"panic recovered",
					slog.String("error", fmt.Sprintf("%v", err)),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("stack_trace", string(stack)),
				)

				// Convert panic to error
				var panicErr error
				switch e := err.(type) {
				case error:
					panicErr = e
				case string:
					panicErr = errors.New(e)
				default:
					panicErr = errors.Errorf("panic: %v", e)
				}

				// Return 500 error using responder
				m.responder.WriteError(r.Context(), w, api.WrapHTTPError(
					panicErr,
					http.StatusInternalServerError,
				))
			}
		}()

		next.ServeHTTP(w, r)
	})
}
