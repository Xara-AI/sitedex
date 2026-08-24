package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/Xara-AI/sitedex/internal/config"
	"github.com/Xara-AI/sitedex/internal/crawler"
	"github.com/Xara-AI/sitedex/internal/export"
	"github.com/Xara-AI/sitedex/internal/extract/content"
	"github.com/Xara-AI/sitedex/internal/index"
)

// exportWriter adapts export.WritePage to the crawler.PageWriter
// interface, so internal/crawler doesn't need to import internal/export
// (it stays a pure orchestration + networking package).
type exportWriter struct{}

func (exportWriter) WritePage(kbDir string, pageURL *url.URL, page *content.Page) (string, error) {
	return export.WritePage(kbDir, pageURL, page)
}

// indexAdapter adapts an *index.DB to the crawler.Indexer interface, for
// the same reason exportWriter exists: internal/crawler stays decoupled
// from internal/index's concrete types.
type indexAdapter struct{ db *index.DB }

func (a indexAdapter) IndexPage(page crawler.PageForIndex, chunks []crawler.ChunkForIndex) error {
	rec := index.PageRecord{
		URL: page.URL, Title: page.Title, Description: page.Description, Lang: page.Lang,
		Hash: page.Hash, CrawledAt: page.CrawledAt, ETag: page.ETag, LastModified: page.LastModified,
	}
	chunkRecs := make([]index.ChunkRecord, len(chunks))
	for i, c := range chunks {
		chunkRecs[i] = index.ChunkRecord{Ordinal: c.Ordinal, HeadingPath: c.HeadingPath, Text: c.Text}
	}
	return a.db.IndexPage(rec, chunkRecs)
}

func (a indexAdapter) IndexProduct(pageURL string, p *crawler.ProductForIndex) error {
	if p == nil {
		return a.db.IndexProduct(pageURL, nil)
	}
	return a.db.IndexProduct(pageURL, &index.ProductRecord{
		Name: p.Name, Description: p.Description, Price: p.Price, HasPrice: p.HasPrice,
		Currency: p.Currency, Availability: p.Availability, Image: p.Image,
		ExtractionMethod: p.ExtractionMethod, RawJSON: p.RawJSON,
	})
}

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

	siteKey, err := crawler.SiteForURL(*site)
	if err != nil {
		return err
	}
	idx, err := index.Open(cfg.DataDir, siteKey)
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer func() { _ = idx.Close() }()

	// A crawl of up to max_pages at rate_limit_rps can run for a while;
	// let Ctrl-C (or a container's SIGTERM) stop it cleanly, saving
	// whatever revalidation state and index writes have accumulated.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c := crawler.New(cfg.Crawl, cfg.Chunking, cfg.DataDir, exportWriter{}, indexAdapter{idx}, func(format string, a ...any) {
		_, _ = fmt.Fprintf(stderr, format+"\n", a...)
	})

	res, err := c.Crawl(ctx, *site)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stdout, "crawl complete: site=%s visited=%d fetched=%d skipped=%d failed=%d duration=%s\n",
		res.Site, res.PagesVisited, res.PagesFetched, res.PagesSkipped, res.PagesFailed, res.Duration.Round(1e6))
	return nil
}
