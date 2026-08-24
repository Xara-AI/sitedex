package server

import (
	"net/http"
	"time"
)

// withAuth requires a matching "Authorization: Bearer <token>" header when
// cfg.Token is set; when it's empty (the default), every request is
// allowed through — per CLAUDE.md, this is deliberately simple, for
// localhost/LXC-internal use. /healthz and /metrics are never gated (see
// routes): infra probes and scrapers shouldn't need a token.
func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	if s.cfg.Token == "" {
		return next
	}
	want := "Bearer " + s.cfg.Token
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != want {
			writeError(w, http.StatusUnauthorized, "invalid or missing bearer token")
			return
		}
		next(w, r)
	}
}

// withLogging logs one structured line per request: method, path, status,
// duration.
func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.logger.Info("http request",
			"method", r.Method, "path", r.URL.Path, "status", rec.status,
			"duration_ms", time.Since(start).Milliseconds())
	})
}

// statusRecorder captures the status code a handler writes, since
// http.ResponseWriter doesn't expose it after the fact.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
