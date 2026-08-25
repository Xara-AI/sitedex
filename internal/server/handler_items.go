package server

import (
	"net/http"
	"os"
	"strconv"

	"github.com/Xara-AI/sitedex/internal/index"
)

// itemDTO is one entry in GET /v1/sites/{site}/items — a page, or (Type ==
// "product") a page's extracted product data layered on top. Fields that
// don't apply to the item's type are omitted rather than sent as zero
// values, same convention as searchResult.
type itemDTO struct {
	URL              string   `json:"url"`
	Type             string   `json:"type"`
	Title            string   `json:"title"`
	Hash             string   `json:"hash,omitempty"`
	CrawledAt        string   `json:"crawled_at,omitempty"`
	Price            *float64 `json:"price,omitempty"`
	Currency         string   `json:"currency,omitempty"`
	Availability     string   `json:"availability,omitempty"`
	ExtractionMethod string   `json:"extraction_method,omitempty"`
	VerifiedAt       string   `json:"verified_at,omitempty"`
	Seq              int64    `json:"seq"`
}

func toItemDTO(it index.ItemRecord) itemDTO {
	out := itemDTO{
		URL: it.URL, Type: it.Type, Title: it.Title, Hash: it.Hash,
		CrawledAt: it.CrawledAt, Currency: it.Currency, Availability: it.Availability,
		ExtractionMethod: it.ExtractionMethod, VerifiedAt: it.VerifiedAt, Seq: it.Seq,
	}
	if it.HasPrice {
		price := it.Price
		out.Price = &price
	}
	return out
}

// handleItems serves GET /v1/sites/{site}/items?since_seq=&limit=&type= — a
// changefeed over one site's indexed pages/products for a consumer that
// wants to know what's in the index and when it was last touched (crawl or
// fresh-verify) without diffing a full listing itself. See index.ListItems
// for the seq/cursor contract. Never crawled yet (or an unknown site) is
// zero items, not an error — consistent with the rest of this API treating
// "nothing indexed" as a normal state, not a failure.
func (s *Server) handleItems(w http.ResponseWriter, r *http.Request) {
	site := r.PathValue("site")
	if site == "" {
		writeError(w, http.StatusBadRequest, "site is required")
		return
	}

	sinceSeq, err := parseInt64Query(r, "since_seq", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "since_seq must be an integer")
		return
	}
	limit, err := parseInt64Query(r, "limit", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "limit must be an integer")
		return
	}
	typeFilter := r.URL.Query().Get("type")

	// A site that's never been crawled has no index.db yet; index.Open
	// would happily create an empty one, which we don't want as a side
	// effect of a read-only GET. Same guard as search.SearchFresh: treat
	// "not indexed" as zero items, not an error.
	if _, err := os.Stat(index.Path(s.cfg.DataDir, site)); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []itemDTO{}, "next_since_seq": sinceSeq})
		return
	}

	idx, err := index.Open(s.cfg.DataDir, site)
	if err != nil {
		s.logger.Error("items: open index failed", "site", site, "err", err)
		writeError(w, http.StatusInternalServerError, "open index failed")
		return
	}
	defer func() { _ = idx.Close() }()

	records, nextSeq, err := idx.ListItems(sinceSeq, typeFilter, int(limit))
	if err != nil {
		s.logger.Error("items: list failed", "site", site, "err", err)
		writeError(w, http.StatusInternalServerError, "list items failed")
		return
	}
	if nextSeq == 0 {
		// No new items this poll: tell the caller to keep using the cursor
		// they already have rather than regressing it to 0.
		nextSeq = sinceSeq
	}

	items := make([]itemDTO, len(records))
	for i, it := range records {
		items[i] = toItemDTO(it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_since_seq": nextSeq})
}

func parseInt64Query(r *http.Request, key string, def int64) (int64, error) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def, nil
	}
	return strconv.ParseInt(v, 10, 64)
}
