// Package server implements the sitedex HTTP API (POST /v1/search, POST
// /v1/crawl, GET /v1/crawl/{job}, GET /v1/sites, GET /healthz, GET
// /metrics) for "sitedex serve" mode, including the fresh-verify response
// budget, a background re-crawl scheduler, and graceful shutdown.
//
// See CLAUDE.md, "HTTP API (serve mode)".
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/Xara-AI/sitedex/internal/config"
	"github.com/Xara-AI/sitedex/internal/search"
)

// Server is the sitedex HTTP API.
type Server struct {
	cfg      *config.Config
	logger   *slog.Logger
	searcher *search.Searcher
	jobs     *jobTracker
	metrics  *metrics
}

// New builds a Server from the effective configuration. A nil logger
// falls back to slog.Default().
func New(cfg *config.Config, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		cfg:      cfg,
		logger:   logger,
		searcher: search.New(cfg.DataDir, cfg.Crawl.UserAgent),
		jobs:     newJobTracker(),
		metrics:  newMetrics(),
	}
}

// Run starts the HTTP server on addr and blocks until ctx is canceled or
// the server fails to start. See serve for shutdown behavior.
func (s *Server) Run(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	return s.serve(ctx, ln)
}

// serve runs the HTTP server on an already-open listener (letting callers
// — tests in particular — choose/discover the address themselves rather
// than parsing it back out of a string) and blocks until ctx is canceled,
// at which point it shuts down gracefully: stop accepting new
// connections, let in-flight requests finish (up to a 15s grace period),
// then return. Background crawl jobs are canceled via ctx too — each
// still saves its crawl state (see crawler.Crawl) before its goroutine
// exits, so an interrupted crawl resumes cheaply (via conditional GET) on
// the next run rather than restarting from scratch.
func (s *Server) serve(ctx context.Context, ln net.Listener) error {
	httpServer := &http.Server{Handler: s.routes(ctx)}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("serve: listening", "addr", ln.Addr().String())
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	stopScheduler := s.startScheduler(ctx)
	defer stopScheduler()

	select {
	case <-ctx.Done():
		s.logger.Info("serve: shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// routes builds the request mux. serveCtx is the server's overall
// lifetime context (canceled on shutdown) — only handlers that spawn
// work outliving the triggering HTTP request (crawl jobs) need it; every
// other handler uses the per-request r.Context() instead, which is
// already canceled on client disconnect and allowed to finish during a
// graceful shutdown.
func (s *Server) routes(serveCtx context.Context) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	mux.HandleFunc("POST /v1/search", s.withAuth(s.handleSearch))
	mux.HandleFunc("POST /v1/crawl", s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		s.handleCrawlTrigger(serveCtx, w, r)
	}))
	mux.HandleFunc("GET /v1/crawl/{job}", s.withAuth(s.handleCrawlStatus))
	mux.HandleFunc("GET /v1/sites", s.withAuth(s.handleSites))
	return s.withLogging(mux)
}
