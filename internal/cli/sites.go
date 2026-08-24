package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/Xara-AI/sitedex/internal/config"
	"github.com/Xara-AI/sitedex/internal/index"
)

func runSites(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("sites", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "", "path to sitedex.yaml (optional)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "Usage: sitedex sites [--config sitedex.yaml]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	sites, err := index.ListSites(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("list sites: %w", err)
	}
	if len(sites) == 0 {
		_, _ = fmt.Fprintf(stdout, "no indexed sites yet under %s — run `sitedex crawl --site <url>` first\n", cfg.DataDir)
		return nil
	}

	_, _ = fmt.Fprintf(stdout, "%-30s %8s %8s %8s  %s\n", "SITE", "PAGES", "CHUNKS", "PRODUCTS", "LAST CRAWLED")
	for _, site := range sites {
		idx, err := index.Open(cfg.DataDir, site)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "sites: open %s: %v\n", site, err)
			continue
		}
		stats, err := idx.Stats()
		_ = idx.Close()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "sites: stats %s: %v\n", site, err)
			continue
		}
		lastCrawled := stats.LastCrawledAt
		if lastCrawled == "" {
			lastCrawled = "-"
		}
		_, _ = fmt.Fprintf(stdout, "%-30s %8d %8d %8d  %s\n", site, stats.PageCount, stats.ChunkCount, stats.ProductCount, lastCrawled)
	}
	return nil
}
