package middleware

import (
	"net/http"
	"runtime/debug"

	"mote/internal/gateway/handlers"
	"mote/pkg/logger"
)

// Recovery returns a middleware that recovers from panics.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				stack := debug.Stack()

				logger.Error().
					Interface("error", err).
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Bytes("stack", stack).
					Msg("panic recovered")

				handlers.SendError(
					w,
					http.StatusInternalServerError,
					handlers.ErrCodeInternalError,
					"internal server error",
				)
			}
		}()

		next.ServeHTTP(w, r)
	})
}
