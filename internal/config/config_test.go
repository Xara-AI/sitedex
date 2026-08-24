package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid, got: %v", err)
	}
	if cfg.DataDir != "./sitedex-data" {
		t.Errorf("DataDir = %q, want ./sitedex-data", cfg.DataDir)
	}
	if cfg.Crawl.RateLimitRPS != 1.0 {
		t.Errorf("Crawl.RateLimitRPS = %v, want 1.0", cfg.Crawl.RateLimitRPS)
	}
	if !cfg.Crawl.RespectRobots {
		t.Errorf("Crawl.RespectRobots = false, want true (polite by default)")
	}
	if time.Duration(cfg.Crawl.RecrawlInterval) != 24*time.Hour {
		t.Errorf("Crawl.RecrawlInterval = %v, want 24h", cfg.Crawl.RecrawlInterval)
	}
	if cfg.Token != "" {
		t.Errorf("Token = %q, want empty (no auth by default)", cfg.Token)
	}
}

func TestLoad_MissingFileFallsBackToDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DataDir != Default().DataDir {
		t.Errorf("expected defaults when file is missing, got DataDir=%q", cfg.DataDir)
	}
}

func TestLoad_EmptyPathUsesDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != Default().Listen {
		t.Errorf("Listen = %q, want default %q", cfg.Listen, Default().Listen)
	}
}

func TestLoad_YAMLOverridesDefaultsPartially(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sitedex.yaml")
	yamlContent := `
data_dir: /var/lib/sitedex
crawl:
  max_pages: 500
  exclude: ["/cart"]
search:
  fresh_top_n: 5
`
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.DataDir != "/var/lib/sitedex" {
		t.Errorf("DataDir = %q, want /var/lib/sitedex", cfg.DataDir)
	}
	if cfg.Crawl.MaxPages != 500 {
		t.Errorf("Crawl.MaxPages = %d, want 500", cfg.Crawl.MaxPages)
	}
	if len(cfg.Crawl.Exclude) != 1 || cfg.Crawl.Exclude[0] != "/cart" {
		t.Errorf("Crawl.Exclude = %v, want [/cart]", cfg.Crawl.Exclude)
	}
	if cfg.Search.FreshTopN != 5 {
		t.Errorf("Search.FreshTopN = %d, want 5", cfg.Search.FreshTopN)
	}
	// Fields not present in YAML keep their defaults.
	if cfg.Crawl.MaxDepth != Default().Crawl.MaxDepth {
		t.Errorf("Crawl.MaxDepth = %d, want default %d (untouched by partial YAML)", cfg.Crawl.MaxDepth, Default().Crawl.MaxDepth)
	}
	if cfg.Listen != Default().Listen {
		t.Errorf("Listen = %q, want default %q (untouched by partial YAML)", cfg.Listen, Default().Listen)
	}
}

func TestLoad_DurationParsing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sitedex.yaml")
	if err := os.WriteFile(path, []byte("crawl:\n  recrawl_interval: 6h\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if time.Duration(cfg.Crawl.RecrawlInterval) != 6*time.Hour {
		t.Errorf("RecrawlInterval = %v, want 6h", cfg.Crawl.RecrawlInterval)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sitedex.yaml")
	if err := os.WriteFile(path, []byte("data_dir: [this is not a string"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestApplyEnv_OverridesYAMLAndDefaults(t *testing.T) {
	t.Setenv("SITEDEX_DATA_DIR", "/env/data")
	t.Setenv("SITEDEX_LISTEN", ":9999")
	t.Setenv("SITEDEX_TOKEN", "secret")
	t.Setenv("SITEDEX_CRAWL_RATE_LIMIT_RPS", "2.5")
	t.Setenv("SITEDEX_CRAWL_MAX_PAGES", "10")
	t.Setenv("SITEDEX_CRAWL_MAX_DEPTH", "3")
	t.Setenv("SITEDEX_CRAWL_RESPECT_ROBOTS", "false")
	t.Setenv("SITEDEX_CRAWL_RECRAWL_INTERVAL", "1h")
	t.Setenv("SITEDEX_SEARCH_FRESH_TOP_N", "1")
	t.Setenv("SITEDEX_SEARCH_AUTO_INDEX_ON_COLD_QUERY", "false")
	t.Setenv("SITEDEX_CHUNKING_TARGET_CHARS", "800")
	t.Setenv("SITEDEX_LLM_EXTRACTOR_PROVIDER", "openai")

	dir := t.TempDir()
	path := filepath.Join(dir, "sitedex.yaml")
	if err := os.WriteFile(path, []byte("data_dir: /yaml/data\ncrawl:\n  max_pages: 500\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.DataDir != "/env/data" {
		t.Errorf("DataDir = %q, want env override /env/data", cfg.DataDir)
	}
	if cfg.Listen != ":9999" {
		t.Errorf("Listen = %q, want :9999", cfg.Listen)
	}
	if cfg.Token != "secret" {
		t.Errorf("Token = %q, want secret", cfg.Token)
	}
	if cfg.Crawl.RateLimitRPS != 2.5 {
		t.Errorf("Crawl.RateLimitRPS = %v, want 2.5", cfg.Crawl.RateLimitRPS)
	}
	if cfg.Crawl.MaxPages != 10 {
		t.Errorf("Crawl.MaxPages = %d, want env override 10 (not YAML's 500)", cfg.Crawl.MaxPages)
	}
	if cfg.Crawl.MaxDepth != 3 {
		t.Errorf("Crawl.MaxDepth = %d, want 3", cfg.Crawl.MaxDepth)
	}
	if cfg.Crawl.RespectRobots {
		t.Errorf("Crawl.RespectRobots = true, want false via env override")
	}
	if time.Duration(cfg.Crawl.RecrawlInterval) != time.Hour {
		t.Errorf("Crawl.RecrawlInterval = %v, want 1h", cfg.Crawl.RecrawlInterval)
	}
	if cfg.Search.FreshTopN != 1 {
		t.Errorf("Search.FreshTopN = %d, want 1", cfg.Search.FreshTopN)
	}
	if cfg.Search.AutoIndexOnColdQuery {
		t.Errorf("Search.AutoIndexOnColdQuery = true, want false via env override")
	}
	if cfg.Chunking.TargetChars != 800 {
		t.Errorf("Chunking.TargetChars = %d, want 800", cfg.Chunking.TargetChars)
	}
	if cfg.LLMExtractor.Provider != "openai" {
		t.Errorf("LLMExtractor.Provider = %q, want openai", cfg.LLMExtractor.Provider)
	}
}

func TestApplyEnv_InvalidValueErrors(t *testing.T) {
	t.Setenv("SITEDEX_CRAWL_MAX_PAGES", "not-a-number")
	if _, err := Load(""); err == nil {
		t.Fatal("expected error for invalid SITEDEX_CRAWL_MAX_PAGES, got nil")
	}
}

func TestValidate_RejectsBadValues(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"empty data dir", func(c *Config) { c.DataDir = "" }, true},
		{"zero rate limit", func(c *Config) { c.Crawl.RateLimitRPS = 0 }, true},
		{"negative max pages", func(c *Config) { c.Crawl.MaxPages = -1 }, true},
		{"zero max depth", func(c *Config) { c.Crawl.MaxDepth = 0 }, true},
		{"negative fresh top n", func(c *Config) { c.Search.FreshTopN = -1 }, true},
		{"zero fresh timeout", func(c *Config) { c.Search.FreshTimeoutMS = 0 }, true},
		{"overlap >= target", func(c *Config) { c.Chunking.OverlapChars = c.Chunking.TargetChars }, true},
		{"bad llm provider", func(c *Config) { c.LLMExtractor.Provider = "gemini" }, true},
		{"valid default", func(c *Config) {}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			tc.mutate(cfg)
			err := cfg.Validate()
			if tc.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}
