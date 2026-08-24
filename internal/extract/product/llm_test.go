package product

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withLLMEndpoint(t *testing.T, provider string, srv *httptest.Server) {
	t.Helper()
	switch provider {
	case "openai":
		orig := openAIURL
		openAIURL = srv.URL
		t.Cleanup(func() { openAIURL = orig })
	case "anthropic":
		orig := anthropicURL
		anthropicURL = srv.URL
		t.Cleanup(func() { anthropicURL = orig })
	}
}

func TestExtractWithLLM_UsesFreeTiersFirst(t *testing.T) {
	// JSON-LD is present, so the LLM tier must never be reached — a call
	// to the (nonexistent) endpoint would fail the test via a network
	// error surfacing as ok=false, so also assert ok=true to catch that.
	raw := loadFixture(t, "jsonld-basic.html")
	p, ok := ExtractWithLLM(context.Background(), raw, mustURL(t, "https://shop.example.com/x"),
		LLMConfig{Provider: "openai", APIKey: "unused"})
	if !ok {
		t.Fatal("expected the JSON-LD tier to succeed without touching the LLM")
	}
	if p.ExtractionMethod != MethodJSONLD {
		t.Errorf("ExtractionMethod = %q, want json-ld (LLM tier should not have been reached)", p.ExtractionMethod)
	}
}

func TestExtractWithLLM_DisabledByDefault(t *testing.T) {
	raw := loadFixture(t, "not-a-product-article.html")
	_, ok := ExtractWithLLM(context.Background(), raw, mustURL(t, "https://example.com/x"), LLMConfig{})
	if ok {
		t.Error("expected no extraction with a zero-value (disabled) LLMConfig")
	}
}

func TestExtractWithLLM_NoAPIKeyDisablesLLMTier(t *testing.T) {
	raw := loadFixture(t, "not-a-product-article.html")
	_, ok := ExtractWithLLM(context.Background(), raw, mustURL(t, "https://example.com/x"),
		LLMConfig{Provider: "openai", APIKey: ""})
	if ok {
		t.Error("expected no extraction when Provider is set but APIKey is empty")
	}
}

func TestExtractWithLLM_OpenAI_Product(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		reply := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"is_product":true,"name":"Blue Mug","description":"A blue mug.","price":12.5,"currency":"USD","availability":"in_stock","image":"/img/mug.jpg"}`}},
			},
		}
		_ = json.NewEncoder(w).Encode(reply)
	}))
	defer srv.Close()
	withLLMEndpoint(t, "openai", srv)

	raw := loadFixture(t, "not-a-product-article.html") // no JSON-LD/microdata/OG/heuristic match
	p, ok := ExtractWithLLM(context.Background(), raw, mustURL(t, "https://shop.example.com/mug"),
		LLMConfig{Provider: "openai", APIKey: "test-key"})
	if !ok {
		t.Fatal("expected the LLM tier to succeed")
	}
	if p.ExtractionMethod != MethodLLM {
		t.Errorf("ExtractionMethod = %q, want llm", p.ExtractionMethod)
	}
	if p.Name != "Blue Mug" {
		t.Errorf("Name = %q", p.Name)
	}
	if !p.HasPrice || p.Price != 12.5 {
		t.Errorf("Price = %v/%v, want 12.5", p.Price, p.HasPrice)
	}
	if p.Availability != InStock {
		t.Errorf("Availability = %q, want in_stock", p.Availability)
	}
	if p.Image != "https://shop.example.com/img/mug.jpg" {
		t.Errorf("Image = %q", p.Image)
	}
}

func TestExtractWithLLM_Anthropic_Product(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("x-api-key = %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("expected anthropic-version header to be set")
		}
		reply := map[string]any{
			"content": []map[string]string{
				{"type": "text", "text": `{"is_product":true,"name":"Red Scarf","description":"","price":25,"currency":"EUR","availability":"unknown","image":""}`},
			},
		}
		_ = json.NewEncoder(w).Encode(reply)
	}))
	defer srv.Close()
	withLLMEndpoint(t, "anthropic", srv)

	raw := loadFixture(t, "not-a-product-article.html")
	p, ok := ExtractWithLLM(context.Background(), raw, mustURL(t, "https://shop.example.com/scarf"),
		LLMConfig{Provider: "anthropic", APIKey: "test-key"})
	if !ok {
		t.Fatal("expected the LLM tier to succeed")
	}
	if p.Name != "Red Scarf" || p.Currency != "EUR" {
		t.Errorf("got Name=%q Currency=%q", p.Name, p.Currency)
	}
	if p.ExtractionMethod != MethodLLM {
		t.Errorf("ExtractionMethod = %q, want llm", p.ExtractionMethod)
	}
}

func TestExtractWithLLM_NotAProduct(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"is_product":false}`}},
			},
		}
		_ = json.NewEncoder(w).Encode(reply)
	}))
	defer srv.Close()
	withLLMEndpoint(t, "openai", srv)

	raw := loadFixture(t, "not-a-product-article.html")
	_, ok := ExtractWithLLM(context.Background(), raw, mustURL(t, "https://example.com/blog/post"),
		LLMConfig{Provider: "openai", APIKey: "test-key"})
	if ok {
		t.Error("expected no product when the model says is_product=false")
	}
}

func TestExtractWithLLM_StripsCodeFenceFromReply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "```json\n{\"is_product\":true,\"name\":\"Fenced Item\"}\n```"}},
			},
		}
		_ = json.NewEncoder(w).Encode(reply)
	}))
	defer srv.Close()
	withLLMEndpoint(t, "openai", srv)

	raw := loadFixture(t, "not-a-product-article.html")
	p, ok := ExtractWithLLM(context.Background(), raw, mustURL(t, "https://example.com/x"),
		LLMConfig{Provider: "openai", APIKey: "test-key"})
	if !ok || p.Name != "Fenced Item" {
		t.Errorf("got p=%+v ok=%v, want Name=Fenced Item", p, ok)
	}
}

func TestExtractWithLLM_APIErrorDegradesGracefully(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()
	withLLMEndpoint(t, "openai", srv)

	raw := loadFixture(t, "not-a-product-article.html")
	_, ok := ExtractWithLLM(context.Background(), raw, mustURL(t, "https://example.com/x"),
		LLMConfig{Provider: "openai", APIKey: "bad-key"})
	if ok {
		t.Error("expected ok=false on an API error, not a panic or propagated error")
	}
}

func TestExtractWithLLM_MalformedJSONReplyDegradesGracefully(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "not valid json at all"}},
			},
		}
		_ = json.NewEncoder(w).Encode(reply)
	}))
	defer srv.Close()
	withLLMEndpoint(t, "openai", srv)

	raw := loadFixture(t, "not-a-product-article.html")
	_, ok := ExtractWithLLM(context.Background(), raw, mustURL(t, "https://example.com/x"),
		LLMConfig{Provider: "openai", APIKey: "test-key"})
	if ok {
		t.Error("expected ok=false when the model's reply isn't valid JSON")
	}
}
