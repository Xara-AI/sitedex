package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/Xara-AI/sitedex/internal/config"
	"github.com/Xara-AI/sitedex/internal/search"
)

func runSearch(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	site := fs.String("site", "", "indexed site to query, e.g. example.com (required)")
	query := fs.String("query", "", "search query text (required)")
	fresh := fs.Bool("fresh", false, "live-verify top results (price/availability) before returning")
	soft := fs.Bool("soft", false, "if the exact query matches nothing, retry with suffix-relaxed (prefix) matching")
	limit := fs.Int("limit", 10, "max results to return")
	configPath := fs.String("config", "", "path to sitedex.yaml (optional)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "Usage: sitedex search --site example.com --query \"blue nike shoes\" [--fresh] [--soft] [--limit 10] [--config sitedex.yaml]\n\nFlags:\n")
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

	if *fresh {
		// TODO(M5): live re-verification of top results within
		// search.fresh_timeout_ms, per CLAUDE.md's "Search" section.
		_, _ = fmt.Fprintln(stderr, "search: --fresh not implemented yet (target milestone M5); returning index-only results")
	}

	searcher := search.New(cfg.DataDir, cfg.Crawl.UserAgent)
	var results []search.Result
	if *soft {
		results, err = searcher.SearchSoft(*site, *query, *limit)
	} else {
		results, err = searcher.Search(*site, *query, *limit)
	}
	if err != nil {
		return err
	}

	if len(results) == 0 {
		_, _ = fmt.Fprintln(stdout, "no results")
		return nil
	}
	for i, r := range results {
		_, _ = fmt.Fprintf(stdout, "%d. [%s] %s  (score %.2f)\n   %s\n", i+1, r.Type, r.Title, r.Score, r.URL)
		if r.Type == "product" {
			priceStr := "price unknown"
			if r.HasPrice {
				priceStr = fmt.Sprintf("%.2f %s", r.Price, r.Currency)
			}
			_, _ = fmt.Fprintf(stdout, "   %s, %s (via %s)\n", priceStr, r.Availability, r.ExtractionMethod)
		}
		if r.HeadingPath != "" {
			_, _ = fmt.Fprintf(stdout, "   %s\n", r.HeadingPath)
		}
		if r.Snippet != "" {
			_, _ = fmt.Fprintf(stdout, "   %s\n", r.Snippet)
		}
	}
	return nil
}
