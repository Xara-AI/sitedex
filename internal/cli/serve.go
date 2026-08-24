package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/Xara-AI/sitedex/internal/config"
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

	// TODO(M5): internal/server HTTP API (search/crawl/sites/healthz/metrics),
	// background re-crawl scheduler, graceful shutdown.
	_, _ = fmt.Fprintf(stdout, "serve: not implemented yet (target milestone M5); listen=%s data_dir=%s\n", cfg.Listen, cfg.DataDir)
	return nil
}
