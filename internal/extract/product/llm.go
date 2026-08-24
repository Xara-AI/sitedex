package product

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// LLMConfig configures the optional last-resort LLM extractor (see
// ExtractWithLLM). Provider "" or "none" (the default) leaves it
// disabled — Extract itself never touches the network.
//
// Implemented as plain net/http calls to the OpenAI and Anthropic REST
// APIs rather than their SDKs: this feature is opt-in and off by default,
// and CLAUDE.md's minimal-dependencies constraint means the two SDKs
// shouldn't become permanent additions to every sitedex binary for a
// feature most users will never enable.
type LLMConfig struct {
	Provider string // "openai" | "anthropic" | "none"/""
	Model    string // provider-specific model ID; empty uses a low-cost default
	APIKey   string // resolved by the caller from the configured api_key_env var
}

func (c LLMConfig) enabled() bool {
	return (c.Provider == "openai" || c.Provider == "anthropic") && c.APIKey != ""
}

const (
	// maxLLMInputChars bounds the page text sent to the model, controlling
	// token cost — a single product's description rarely needs more than
	// this to be identifiable.
	maxLLMInputChars = 6000
	llmTimeout       = 30 * time.Second

	defaultOpenAIModel    = "gpt-4o-mini"
	defaultAnthropicModel = "claude-haiku-4-5"
)

// openAIURL and anthropicURL are package variables (rather than inline
// constants) specifically so tests can point them at a local httptest
// server instead of the real APIs.
var (
	openAIURL    = "https://api.openai.com/v1/chat/completions"
	anthropicURL = "https://api.anthropic.com/v1/messages"
)

// llmSystemPrompt is the strict-schema prompt CLAUDE.md calls for: "Takes
// stripped HTML, returns the same product JSON via a strict schema
// prompt."
const llmSystemPrompt = `You are a product-data extraction assistant. You will be given the visible text of one web page. Determine whether the page is a single product's detail page (one specific item for sale, with its own price) — not an article, a category/search listing, a cart, or a home page.

Respond with ONLY a single JSON object and nothing else: no markdown code fences, no commentary, no explanation. Match exactly one of these two shapes.

If the page IS a single product's detail page:
{"is_product": true, "name": "string", "description": "string", "price": number or null, "currency": "3-letter ISO 4217 code such as USD, EUR, RON, or empty string if unknown", "availability": "in_stock" or "out_of_stock" or "unknown", "image": "absolute image URL or empty string"}

If it is NOT:
{"is_product": false}`

// ExtractWithLLM runs Extract's four free tiers (JSON-LD -> microdata ->
// OpenGraph -> CSS heuristics) first, then — only if all four fail and
// llm is enabled (Provider set and an API key resolved) — calls out to
// the configured LLM as a last resort. Extract alone never makes a
// network call; this is the only entry point that does, matching
// CLAUDE.md: "This is a pluggable last resort, not the engine."
func ExtractWithLLM(ctx context.Context, raw []byte, pageURL *url.URL, llm LLMConfig) (*Product, bool) {
	if p, ok := Extract(raw, pageURL); ok {
		return p, true
	}
	if !llm.enabled() {
		return nil, false
	}
	return extractLLM(ctx, raw, pageURL, llm)
}

func extractLLM(ctx context.Context, raw []byte, pageURL *url.URL, llm LLMConfig) (*Product, bool) {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, false
	}

	title := ""
	if t := findByTag(doc, "title"); t != nil {
		title = textContent(t)
	}
	body := truncateRunes(textContent(doc), maxLLMInputChars)
	if body == "" {
		return nil, false
	}

	ctx, cancel := context.WithTimeout(ctx, llmTimeout)
	defer cancel()

	var reply string
	switch llm.Provider {
	case "openai":
		reply, err = callOpenAI(ctx, llm, strings.TrimSpace(title+"\n\n"+body))
	case "anthropic":
		reply, err = callAnthropic(ctx, llm, strings.TrimSpace(title+"\n\n"+body))
	default:
		return nil, false
	}
	if err != nil {
		return nil, false
	}

	return parseLLMReply(reply, pageURL)
}

var llmHTTPClient = &http.Client{}

func callOpenAI(ctx context.Context, llm LLMConfig, userContent string) (string, error) {
	model := llm.Model
	if model == "" {
		model = defaultOpenAIModel
	}
	reqBody, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": llmSystemPrompt},
			{"role": "user", "content": userContent},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+llm.APIKey)
	req.Header.Set("Content-Type", "application/json")

	data, err := doLLMRequest(req)
	if err != nil {
		return "", err
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("openai: decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openai: no choices in response")
	}
	return parsed.Choices[0].Message.Content, nil
}

func callAnthropic(ctx context.Context, llm LLMConfig, userContent string) (string, error) {
	model := llm.Model
	if model == "" {
		model = defaultAnthropicModel
	}
	reqBody, err := json.Marshal(map[string]any{
		"model":      model,
		"max_tokens": 1024,
		"system":     llmSystemPrompt,
		"messages":   []map[string]string{{"role": "user", "content": userContent}},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", llm.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	data, err := doLLMRequest(req)
	if err != nil {
		return "", err
	}

	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("anthropic: decode response: %w", err)
	}
	for _, c := range parsed.Content {
		if c.Type == "text" && c.Text != "" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic: no text content in response")
}

func doLLMRequest(req *http.Request) ([]byte, error) {
	resp, err := llmHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, truncateRunes(string(data), 500))
	}
	return data, nil
}

type llmProductReply struct {
	IsProduct    bool     `json:"is_product"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Price        *float64 `json:"price"`
	Currency     string   `json:"currency"`
	Availability string   `json:"availability"`
	Image        string   `json:"image"`
}

// parseLLMReply tolerantly parses the model's response: models
// occasionally wrap JSON in a markdown code fence despite instructions
// not to, so that's stripped defensively before unmarshaling.
func parseLLMReply(reply string, pageURL *url.URL) (*Product, bool) {
	reply = stripCodeFence(reply)

	var v llmProductReply
	if err := json.Unmarshal([]byte(reply), &v); err != nil {
		return nil, false
	}
	if !v.IsProduct || v.Name == "" {
		return nil, false
	}

	p := &Product{
		URL: pageURL.String(), Name: normalizeSpace(v.Name), Description: normalizeSpace(v.Description),
		Currency: v.Currency, ExtractionMethod: MethodLLM,
		Availability: availabilityField(v.Availability),
	}
	if v.Price != nil {
		p.Price, p.HasPrice = *v.Price, true
	}
	if v.Image != "" {
		p.Image = resolveURL(pageURL, v.Image)
	}
	return p, true
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
