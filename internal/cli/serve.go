package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Xara-AI/sitedex/internal/config"
	"github.com/Xara-AI/sitedex/internal/server"
)

func runServe(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "", "listen address, e.g. :8080 (overrides config listen)")
	configPath := fs.String("config", "", "path to sitedex.yaml (optional)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "Usage: sitedex serve [--addr :8080] [--config sitedex.yaml]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *addr != "" {
		cfg.Listen = *addr
	}

	// Structured JSON lines to stdout, per CLAUDE.md's deployment section
	// (journald/docker-logs friendly, no log files).
	logger := slog.New(slog.NewJSONHandler(stdout, &slog.HandlerOptions{Level: logLevel(cfg.LogLevel)}))

	// SIGTERM is what container orchestrators (systemd, Docker, k8s) send
	// on stop/restart; SIGINT is Ctrl-C for local runs.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return server.New(cfg, logger).Run(ctx, cfg.Listen)
}

func logLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
