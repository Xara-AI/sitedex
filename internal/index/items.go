package index

import (
	"database/sql"
	"fmt"
)

// ItemRecord is one page-level entry in the index introspection feed (GET
// /v1/sites/{site}/items): a page, or — if the page also has a product
// record — that product's data layered on top. This mirrors Search's "the
// product representation wins" rule (see index/search.go) so the two
// endpoints describe the same site consistently.
//
// Seq is the row's position in this site's write-order changefeed (see
// seq.go): the effective seq of an item is the higher of its page and
// product seq, so an item resurfaces in a since_seq poll whenever *either*
// its page metadata (a re-crawl) or its product data (a fresh-verify)
// last changed — not just on a full crawl.
type ItemRecord struct {
	URL       string
	Type      string // "page" or "product"
	Title     string
	Hash      string
	CrawledAt string // RFC3339, from pages.crawled_at
	ETag      string
	Seq       int64

	// Product fields, populated only when Type == "product".
	Price            float64
	HasPrice         bool
	Currency         string
	Availability     string
	ExtractionMethod string
	VerifiedAt       string // RFC3339, empty if never fresh-verified
}

// ListItems returns up to limit items with effective seq > sinceSeq,
// ordered oldest-changed-first, plus the highest seq among them (0 if none
// — callers should keep polling with their previous since_seq in that
// case, not regress it). typeFilter is "", "any", "page", or "product",
// matching SearchFiltered's convention.
//
// This is a changefeed, not a snapshot listing: a poller that starts at
// since_seq=0 and keeps calling with the returned nextSeq eventually sees
// every item at least once, and sees an item again only when it actually
// changed — the right shape for a consumer that wants to stay in sync
// without re-fetching the whole index on every poll.
func (db *DB) ListItems(sinceSeq int64, typeFilter string, limit int) (items []ItemRecord, nextSeq int64, err error) {
	if limit <= 0 {
		limit = 200
	}

	typeCond := ""
	switch typeFilter {
	case "page":
		typeCond = "AND pr.page_url IS NULL"
	case "product":
		typeCond = "AND pr.page_url IS NOT NULL"
	}

	// The LEFT JOIN is 1:0-or-1 (products.page_url is a PK), so this is
	// already one row per page — no GROUP BY needed, MAX(a,b) here is
	// SQLite's two-arg scalar max, not the aggregate.
	rows, err := db.sql.Query(fmt.Sprintf(`
		SELECT p.url, p.title, p.hash, p.crawled_at, p.etag,
		       pr.page_url IS NOT NULL AS is_product,
		       pr.name, pr.price, pr.currency, pr.availability, pr.extraction_method, pr.verified_at,
		       MAX(p.seq, COALESCE(pr.seq, 0)) AS eff_seq
		FROM pages p
		LEFT JOIN products pr ON pr.page_url = p.url
		WHERE MAX(p.seq, COALESCE(pr.seq, 0)) > ?
		%s
		ORDER BY eff_seq ASC
		LIMIT ?
	`, typeCond), sinceSeq, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var it ItemRecord
		var isProduct bool
		var name, currency, availability, method, verifiedAt sql.NullString
		var price sql.NullFloat64
		if err := rows.Scan(&it.URL, &it.Title, &it.Hash, &it.CrawledAt, &it.ETag,
			&isProduct, &name, &price, &currency, &availability, &method, &verifiedAt, &it.Seq); err != nil {
			return nil, 0, fmt.Errorf("scan item row: %w", err)
		}
		if isProduct {
			it.Type = "product"
			it.Title = name.String // product name takes precedence over page title, same as Search
			it.Price = price.Float64
			it.HasPrice = price.Valid
			it.Currency = currency.String
			it.Availability = availability.String
			it.ExtractionMethod = method.String
			it.VerifiedAt = verifiedAt.String
		} else {
			it.Type = "page"
		}
		items = append(items, it)
		if it.Seq > nextSeq {
			nextSeq = it.Seq
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("item rows: %w", err)
	}
	return items, nextSeq, nil
}
