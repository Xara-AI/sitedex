package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/Xara-AI/sitedex/internal/config"
)

func runCrawl(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("crawl", flag.ContinueOnError)
	fs.SetOutput(stderr)
	site := fs.String("site", "", "site URL to crawl, e.g. https://example.com (required)")
	configPath := fs.String("config", "", "path to sitedex.yaml (optional)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "Usage: sitedex crawl --site https://example.com [--config sitedex.yaml]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *site == "" {
		fs.Usage()
		return fmt.Errorf("--site is required")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	// TODO(M2): frontier + robots.txt + rate limiter + fetcher, content
	// extraction, and markdown export land in internal/crawler,
	// internal/extract/content, and internal/export.
	_, _ = fmt.Fprintf(stdout, "crawl: not implemented yet (target milestone M2); site=%s data_dir=%s\n", *site, cfg.DataDir)
	return nil
}
