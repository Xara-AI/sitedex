package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestServer_Items(t *testing.T) {
	dataDir := t.TempDir()
	seedTestProduct(t, dataDir, "example.com", "https://example.com/shoes", "Blue Shoes", 99, "in_stock")

	base, stop := testServer(t, testConfig(dataDir))
	defer stop()

	resp, err := http.Get(base + "/v1/sites/example.com/items")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var out struct {
		Items        []itemDTO `json:"items"`
		NextSinceSeq int64     `json:"next_since_seq"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].Type != "product" || out.Items[0].Title != "Blue Shoes" {
		t.Fatalf("items = %+v", out.Items)
	}
	if out.NextSinceSeq == 0 {
		t.Error("next_since_seq = 0, want non-zero after one write")
	}
}

func TestServer_Items_SinceSeqIsIncremental(t *testing.T) {
	dataDir := t.TempDir()
	seedTestProduct(t, dataDir, "example.com", "https://example.com/shoes", "Blue Shoes", 99, "in_stock")

	base, stop := testServer(t, testConfig(dataDir))
	defer stop()

	first := getItems(t, base, "example.com", 0)
	if len(first.Items) != 1 {
		t.Fatalf("first poll items = %+v, want 1", first.Items)
	}

	// Polling again from the cursor we were handed should come back empty,
	// and echo the same cursor back rather than regressing to 0.
	second := getItems(t, base, "example.com", first.NextSinceSeq)
	if len(second.Items) != 0 {
		t.Fatalf("second poll items = %+v, want none (nothing changed)", second.Items)
	}
	if second.NextSinceSeq != first.NextSinceSeq {
		t.Errorf("next_since_seq = %d, want unchanged %d when nothing new", second.NextSinceSeq, first.NextSinceSeq)
	}

	seedTestProduct(t, dataDir, "example.com", "https://example.com/hats", "Red Hat", 20, "in_stock")
	third := getItems(t, base, "example.com", first.NextSinceSeq)
	if len(third.Items) != 1 || third.Items[0].Title != "Red Hat" {
		t.Fatalf("third poll items = %+v, want just the new product", third.Items)
	}
}

func TestServer_Items_UnknownSiteIsEmptyNotError(t *testing.T) {
	base, stop := testServer(t, testConfig(t.TempDir()))
	defer stop()

	out := getItems(t, base, "never-crawled.example.com", 0)
	if len(out.Items) != 0 {
		t.Errorf("items = %+v, want none for an unindexed site", out.Items)
	}
}

type itemsResponse struct {
	Items        []itemDTO `json:"items"`
	NextSinceSeq int64     `json:"next_since_seq"`
}

func getItems(t *testing.T, base, site string, sinceSeq int64) itemsResponse {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/v1/sites/%s/items?since_seq=%d", base, site, sinceSeq))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out itemsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}
