package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/Xara-AI/sitedex/internal/config"
)

func runSearch(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	site := fs.String("site", "", "indexed site to query, e.g. example.com (required)")
	query := fs.String("query", "", "search query text (required)")
	fresh := fs.Bool("fresh", false, "live-verify top results (price/availability) before returning")
	limit := fs.Int("limit", 10, "max results to return")
	configPath := fs.String("config", "", "path to sitedex.yaml (optional)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "Usage: sitedex search --site example.com --query \"blue nike shoes\" [--fresh] [--limit 10] [--config sitedex.yaml]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *site == "" || *query == "" {
		fs.Usage()
		return fmt.Errorf("--site and --query are required")
	}
	if *limit <= 0 {
		fs.Usage()
		return fmt.Errorf("--limit must be > 0, got %d", *limit)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	// TODO(M3): FTS5 warm-path search in internal/search. TODO(M5): --fresh
	// live verification against cfg.Search.FreshTimeoutMS budget.
	_, _ = fmt.Fprintf(stdout, "search: not implemented yet (target milestone M3); site=%s query=%q fresh=%v limit=%d data_dir=%s\n", *site, *query, *fresh, *limit, cfg.DataDir)
	return nil
}
