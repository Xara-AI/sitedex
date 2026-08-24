package server

import (
	"net/url"

	"github.com/Xara-AI/sitedex/internal/crawler"
	"github.com/Xara-AI/sitedex/internal/export"
	"github.com/Xara-AI/sitedex/internal/extract/content"
	"github.com/Xara-AI/sitedex/internal/index"
)

// exportWriter and indexAdapter mirror internal/cli's crawl wiring
// exactly (see internal/cli/crawl.go) — both entry points (cli, server)
// own their own adapters from crawler's decoupling interfaces to the
// concrete export/index packages, rather than internal/crawler depending
// on either directly.
type exportWriter struct{}

func (exportWriter) WritePage(kbDir string, pageURL *url.URL, page *content.Page) (string, error) {
	return export.WritePage(kbDir, pageURL, page)
}

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
