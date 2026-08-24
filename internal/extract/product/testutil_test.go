package product

import (
	"bytes"
	"testing"

	"golang.org/x/net/html"
)

func mustParseDoc(t *testing.T, raw []byte) *html.Node {
	t.Helper()
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("html.Parse: %v", err)
	}
	return doc
}
