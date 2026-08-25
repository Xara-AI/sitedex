package index

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// preSeqSchema is the pages/products shape as it existed before the "seq"
// (and products.verified_at) columns were added — reproducing exactly what
// a real site indexed under an older sitedex build looks like on disk, so
// TestUpgrade_BackfillsSeqForPreExistingRows exercises the real upgrade
// path rather than a synthetic one.
const preSeqSchema = `
CREATE TABLE pages (
    url           TEXT PRIMARY KEY,
    title         TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    lang          TEXT NOT NULL DEFAULT '',
    hash          TEXT NOT NULL DEFAULT '',
    crawled_at    TEXT NOT NULL DEFAULT '',
    etag          TEXT NOT NULL DEFAULT '',
    last_modified TEXT NOT NULL DEFAULT ''
);
CREATE TABLE products (
    page_url           TEXT PRIMARY KEY REFERENCES pages(url) ON DELETE CASCADE,
    name               TEXT NOT NULL DEFAULT '',
    description        TEXT NOT NULL DEFAULT '',
    price              REAL,
    currency           TEXT NOT NULL DEFAULT '',
    availability       TEXT NOT NULL DEFAULT '',
    image              TEXT NOT NULL DEFAULT '',
    extraction_method  TEXT NOT NULL DEFAULT '',
    raw_json           TEXT NOT NULL DEFAULT ''
);
`

// TestUpgrade_BackfillsSeqForPreExistingRows reproduces the xara.bot
// production bug (2026-08-25): GET /v1/sites reported pages crawled under
// the pre-seq binary, but GET /v1/sites/{site}/items returned nothing for
// any since_seq/type, because rows written before the "seq" column existed
// got seq=0 from the ALTER TABLE default — indistinguishable from
// ListItems' own "not yet seen" bootstrap value (since_seq=0), so they
// were filtered out of every possible query, permanently, without a
// backfill.
func TestUpgrade_BackfillsSeqForPreExistingRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.db")

	// Simulate a site indexed under the old (pre-seq) binary: create the
	// old schema directly and insert rows exactly as IndexPage/IndexProduct
	// would have, minus the seq column that didn't exist yet.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	if _, err := raw.Exec(preSeqSchema); err != nil {
		t.Fatalf("create pre-seq schema: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO pages (url, title, crawled_at) VALUES (?, ?, ?)`,
		"https://xara.bot/", "Home", "2026-08-24T10:00:00Z"); err != nil {
		t.Fatalf("insert legacy page: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO pages (url, title, crawled_at) VALUES (?, ?, ?)`,
		"https://xara.bot/shop", "Shop", "2026-08-24T10:05:00Z"); err != nil {
		t.Fatalf("insert legacy page: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO products (page_url, name, price, currency) VALUES (?, ?, ?, ?)`,
		"https://xara.bot/shop", "Widget", 9.99, "USD"); err != nil {
		t.Fatalf("insert legacy product: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	// Now open it with the real (post-seq) migrate path.
	db, err := open(path)
	if err != nil {
		t.Fatalf("Open (upgrade): %v", err)
	}
	defer func() { _ = db.Close() }()

	stats, err := db.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.PageCount != 2 || stats.ProductCount != 1 {
		t.Fatalf("Stats = %+v, want 2 pages / 1 product to survive the upgrade untouched", stats)
	}

	// This is the actual bug: before the backfill, this returned zero
	// items no matter what since_seq/type was passed, even though Stats
	// (and GET /v1/sites) correctly showed the crawled data.
	items, next, err := db.ListItems(0, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("ListItems(0, ...) = %d items, want 2 (legacy pre-seq rows must be visible after upgrade)", len(items))
	}
	if next == 0 {
		t.Error("next_since_seq = 0 after a non-empty listing, want > 0")
	}
	for _, it := range items {
		if it.Seq <= 0 {
			t.Errorf("item %q has seq = %d, want a real backfilled seq > 0", it.URL, it.Seq)
		}
	}

	// The product row must have gotten a real seq too, not just the page.
	var productSeq int64
	if err := db.sql.QueryRow(`SELECT seq FROM products WHERE page_url = ?`, "https://xara.bot/shop").Scan(&productSeq); err != nil {
		t.Fatal(err)
	}
	if productSeq <= 0 {
		t.Errorf("products.seq = %d after upgrade, want > 0", productSeq)
	}

	// A caller polling from "0" a second time (post-upgrade, nothing new
	// written) should now see the same items again if it restarts its
	// cursor from scratch, and see nothing new if it correctly remembers
	// next_since_seq from this call.
	again, next2, err := db.ListItems(next, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 || next2 != 0 {
		t.Errorf("ListItems(next, ...) = %d items, next = %d, want none (nothing changed since the backfill)", len(again), next2)
	}
}
