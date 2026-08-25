package index

import (
	"database/sql"
	"fmt"
	"time"
)

// ProductRecord is one page's extracted product, as stored in
// products/products_fts.
type ProductRecord struct {
	Name             string
	Description      string
	Price            float64
	HasPrice         bool
	Currency         string
	Availability     string
	Image            string
	ExtractionMethod string
	RawJSON          string

	// VerifiedAt is set only by the search package's fresh-verify path
	// (see search.verifyTop): the moment this product data was last
	// confirmed live against the site, as opposed to CrawledAt on the
	// owning page (which only reflects the last full crawl). Zero means
	// "never fresh-verified" — the common case for a plain crawl-time
	// write.
	VerifiedAt time.Time
}

// IndexProduct upserts (p != nil) or removes (p == nil) the product record
// for pageURL, keeping products_fts in sync. Removing is the correct call
// when a re-crawled page no longer looks like a product (markup changed,
// item delisted, ...) — it prevents stale product data from lingering in
// search results.
func (db *DB) IndexProduct(pageURL string, p *ProductRecord) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := deleteProductRows(tx, pageURL); err != nil {
		return err
	}

	if p == nil {
		return tx.Commit()
	}

	var price sql.NullFloat64
	if p.HasPrice {
		price = sql.NullFloat64{Float64: p.Price, Valid: true}
	}
	var verifiedAt string
	if !p.VerifiedAt.IsZero() {
		verifiedAt = p.VerifiedAt.UTC().Format(time.RFC3339)
	}

	seq, err := nextSeq(tx)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT INTO products (page_url, name, description, price, currency, availability, image, extraction_method, raw_json, verified_at, seq)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(page_url) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			price = excluded.price,
			currency = excluded.currency,
			availability = excluded.availability,
			image = excluded.image,
			extraction_method = excluded.extraction_method,
			raw_json = excluded.raw_json,
			verified_at = excluded.verified_at,
			seq = excluded.seq
	`, pageURL, p.Name, p.Description, price, p.Currency, p.Availability, p.Image, p.ExtractionMethod, p.RawJSON, verifiedAt, seq); err != nil {
		return fmt.Errorf("upsert product: %w", err)
	}

	if _, err := tx.Exec(`INSERT INTO products_fts (page_url, name, description) VALUES (?, ?, ?)`,
		pageURL, p.Name, p.Description); err != nil {
		return fmt.Errorf("insert product fts: %w", err)
	}

	return tx.Commit()
}

func deleteProductRows(tx *sql.Tx, pageURL string) error {
	if _, err := tx.Exec(`DELETE FROM products_fts WHERE page_url = ?`, pageURL); err != nil {
		return fmt.Errorf("delete product fts: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM products WHERE page_url = ?`, pageURL); err != nil {
		return fmt.Errorf("delete product: %w", err)
	}
	return nil
}
