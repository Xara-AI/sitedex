package crawler

import "testing"

func TestFrontier_BFSOrder(t *testing.T) {
	seed := mustParseURL(t, "https://example.com/")
	f := NewFrontier(seed, 5, nil, nil)

	f.Add(seed, 0)
	f.Add(mustParseURL(t, "https://example.com/a"), 1)
	f.Add(mustParseURL(t, "https://example.com/b"), 1)

	var order []string
	for {
		u, _, ok := f.Next()
		if !ok {
			break
		}
		order = append(order, u.String())
	}
	want := []string{"https://example.com/", "https://example.com/a", "https://example.com/b"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order[%d] = %q, want %q", i, order[i], want[i])
		}
	}
}

func TestFrontier_DedupesAcrossAdds(t *testing.T) {
	seed := mustParseURL(t, "https://example.com/")
	f := NewFrontier(seed, 5, nil, nil)

	if !f.Add(mustParseURL(t, "https://example.com/x"), 1) {
		t.Fatal("first Add should succeed")
	}
	if f.Add(mustParseURL(t, "https://example.com/x"), 1) {
		t.Error("second Add of same URL should be rejected as duplicate")
	}
	if f.Len() != 1 {
		t.Errorf("Len() = %d, want 1", f.Len())
	}
}

func TestFrontier_RespectsMaxDepth(t *testing.T) {
	seed := mustParseURL(t, "https://example.com/")
	f := NewFrontier(seed, 2, nil, nil)

	if !f.Add(mustParseURL(t, "https://example.com/depth2"), 2) {
		t.Error("depth == maxDepth should be added")
	}
	if f.Add(mustParseURL(t, "https://example.com/depth3"), 3) {
		t.Error("depth > maxDepth should be rejected")
	}
}

func TestFrontier_ScopesToRegistrableDomain(t *testing.T) {
	seed := mustParseURL(t, "https://shop.example.com/")
	f := NewFrontier(seed, 5, nil, nil)

	if !f.Add(mustParseURL(t, "https://blog.example.com/post"), 1) {
		t.Error("subdomain sharing the registrable domain should be in scope")
	}
	if f.Add(mustParseURL(t, "https://other.com/x"), 1) {
		t.Error("different registrable domain should be out of scope")
	}
}

func TestFrontier_IncludeExcludePatterns(t *testing.T) {
	seed := mustParseURL(t, "https://example.com/")
	f := NewFrontier(seed, 5, []string{`/products/`}, []string{`/cart`})

	if !f.Add(mustParseURL(t, "https://example.com/products/shoes"), 1) {
		t.Error("URL matching include pattern should be added")
	}
	if f.Add(mustParseURL(t, "https://example.com/about"), 1) {
		t.Error("URL not matching include pattern should be rejected")
	}
	if f.Add(mustParseURL(t, "https://example.com/products/cart"), 1) {
		t.Error("URL matching exclude pattern should be rejected even if it also matches include")
	}
}

func TestFrontier_InvalidPatternIsSkippedNotFatal(t *testing.T) {
	seed := mustParseURL(t, "https://example.com/")
	f := NewFrontier(seed, 5, []string{`(unclosed`}, nil)
	// Invalid include pattern is dropped, so include list is effectively
	// empty -> everything in scope passes.
	if !f.Add(mustParseURL(t, "https://example.com/x"), 1) {
		t.Error("invalid include pattern should be ignored, not block everything")
	}
}
