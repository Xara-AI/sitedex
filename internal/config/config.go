// Package config loads sitedex configuration from a YAML file with
// SITEDEX_* environment variable overrides layered on top (env-first,
// intended for container/LXC deployment where env vars are the natural
// override surface). See CLAUDE.md for the full documented schema.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Xara-AI/sitedex/internal/version"
)

// Duration wraps time.Duration so it can be parsed from a YAML string such
// as "24h" (the stdlib encoding, based on the int64 nanosecond count,
// cannot round-trip through YAML scalars on its own).
type Duration time.Duration

// UnmarshalYAML implements yaml.v3's node-based unmarshaler.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

// MarshalYAML implements yaml.Marshaler.
func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

func (d Duration) String() string { return time.Duration(d).String() }

// Config is the root sitedex configuration, mirroring sitedex.yaml.
type Config struct {
	DataDir  string `yaml:"data_dir"`
	Listen   string `yaml:"listen"`
	Token    string `yaml:"token"`     // empty = no auth
	LogLevel string `yaml:"log_level"` // debug|info|warn|error; structured JSON lines to stdout

	Crawl        CrawlConfig        `yaml:"crawl"`
	Search       SearchConfig       `yaml:"search"`
	Chunking     ChunkingConfig     `yaml:"chunking"`
	LLMExtractor LLMExtractorConfig `yaml:"llm_extractor"`
}

// CrawlConfig controls crawler politeness and scope.
type CrawlConfig struct {
	RateLimitRPS  float64  `yaml:"rate_limit_rps"`
	MaxPages      int      `yaml:"max_pages"`
	MaxDepth      int      `yaml:"max_depth"`
	RespectRobots bool     `yaml:"respect_robots"` // override only for sites you own
	UserAgent     string   `yaml:"user_agent"`
	Include       []string `yaml:"include"` // URL regex allowlist, empty = all
	Exclude       []string `yaml:"exclude"` // URL regex denylist

	RecrawlInterval Duration `yaml:"recrawl_interval"`

	// RendererURL is a documented hook for an external JS-rendering service.
	// sitedex v1 does not render JS itself and does not implement one; if
	// set, a future version may call out to it. Empty by default.
	RendererURL string `yaml:"renderer_url"`
}

// SearchConfig controls warm/fresh/cold search behavior.
type SearchConfig struct {
	FreshTopN            int  `yaml:"fresh_top_n"`
	FreshTimeoutMS       int  `yaml:"fresh_timeout_ms"`
	AutoIndexOnColdQuery bool `yaml:"auto_index_on_cold_query"`
}

// ChunkingConfig controls markdown chunk sizing for content extraction.
type ChunkingConfig struct {
	TargetChars  int `yaml:"target_chars"`
	OverlapChars int `yaml:"overlap_chars"`
}

// LLMExtractorConfig controls the optional, disabled-by-default LLM product
// extractor (last resort in the extraction priority chain).
type LLMExtractorConfig struct {
	Provider  string `yaml:"provider"` // none|openai|anthropic
	Model     string `yaml:"model"`
	APIKeyEnv string `yaml:"api_key_env"`
}

// Default returns the built-in default configuration, matching the schema
// documented in CLAUDE.md.
func Default() *Config {
	return &Config{
		DataDir:  "./sitedex-data",
		Listen:   ":8080",
		Token:    "",
		LogLevel: "info",
		Crawl: CrawlConfig{
			RateLimitRPS:    1.0,
			MaxPages:        2000,
			MaxDepth:        5,
			RespectRobots:   true,
			UserAgent:       version.UserAgent(),
			Include:         nil,
			Exclude:         []string{"/cart", "/checkout", "/wp-admin"},
			RecrawlInterval: Duration(24 * time.Hour),
			RendererURL:     "",
		},
		Search: SearchConfig{
			FreshTopN:            3,
			FreshTimeoutMS:       2500,
			AutoIndexOnColdQuery: true,
		},
		Chunking: ChunkingConfig{
			TargetChars:  1200,
			OverlapChars: 100,
		},
		LLMExtractor: LLMExtractorConfig{
			Provider:  "none",
			Model:     "",
			APIKeyEnv: "",
		},
	}
}

// Load builds the effective configuration: defaults, overlaid with the YAML
// file at path (if path is non-empty and the file exists), overlaid with
// SITEDEX_* environment variables. path may be empty to skip file loading.
func Load(path string) (*Config, error) {
	cfg := Default()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("read config %s: %w", path, err)
			}
		} else {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parse config %s: %w", path, err)
			}
		}
	}

	if err := applyEnv(cfg); err != nil {
		return nil, fmt.Errorf("apply env overrides: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate performs basic sanity checks on the effective configuration.
func (c *Config) Validate() error {
	switch {
	case c.DataDir == "":
		return fmt.Errorf("data_dir must not be empty")
	case c.Crawl.RateLimitRPS <= 0:
		return fmt.Errorf("crawl.rate_limit_rps must be > 0")
	case c.Crawl.MaxPages <= 0:
		return fmt.Errorf("crawl.max_pages must be > 0")
	case c.Crawl.MaxDepth <= 0:
		return fmt.Errorf("crawl.max_depth must be > 0")
	case c.Search.FreshTopN < 0:
		return fmt.Errorf("search.fresh_top_n must be >= 0")
	case c.Search.FreshTimeoutMS <= 0:
		return fmt.Errorf("search.fresh_timeout_ms must be > 0")
	case c.Chunking.TargetChars <= 0:
		return fmt.Errorf("chunking.target_chars must be > 0")
	case c.Chunking.OverlapChars < 0:
		return fmt.Errorf("chunking.overlap_chars must be >= 0")
	case c.Chunking.OverlapChars >= c.Chunking.TargetChars:
		return fmt.Errorf("chunking.overlap_chars must be < chunking.target_chars")
	case c.LLMExtractor.Provider != "none" && c.LLMExtractor.Provider != "openai" && c.LLMExtractor.Provider != "anthropic":
		return fmt.Errorf("llm_extractor.provider must be one of none|openai|anthropic, got %q", c.LLMExtractor.Provider)
	case !validLogLevels[c.LogLevel]:
		return fmt.Errorf("log_level must be one of debug|info|warn|error, got %q", c.LogLevel)
	}
	return nil
}

var validLogLevels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}

// envSpec binds one SITEDEX_* environment variable to a setter applied when
// the variable is present (including empty-string values, which are a
// deliberate override).
type envSpec struct {
	key string
	set func(v string) error
}

func applyEnv(cfg *Config) error {
	specs := []envSpec{
		{"SITEDEX_DATA_DIR", func(v string) error { cfg.DataDir = v; return nil }},
		{"SITEDEX_LISTEN", func(v string) error { cfg.Listen = v; return nil }},
		{"SITEDEX_TOKEN", func(v string) error { cfg.Token = v; return nil }},
		{"SITEDEX_LOG_LEVEL", func(v string) error { cfg.LogLevel = v; return nil }},

		{"SITEDEX_CRAWL_RATE_LIMIT_RPS", floatSetter(&cfg.Crawl.RateLimitRPS)},
		{"SITEDEX_CRAWL_MAX_PAGES", intSetter(&cfg.Crawl.MaxPages)},
		{"SITEDEX_CRAWL_MAX_DEPTH", intSetter(&cfg.Crawl.MaxDepth)},
		{"SITEDEX_CRAWL_RESPECT_ROBOTS", boolSetter(&cfg.Crawl.RespectRobots)},
		{"SITEDEX_CRAWL_USER_AGENT", func(v string) error { cfg.Crawl.UserAgent = v; return nil }},
		{"SITEDEX_CRAWL_RECRAWL_INTERVAL", durationSetter(&cfg.Crawl.RecrawlInterval)},
		{"SITEDEX_CRAWL_RENDERER_URL", func(v string) error { cfg.Crawl.RendererURL = v; return nil }},

		{"SITEDEX_SEARCH_FRESH_TOP_N", intSetter(&cfg.Search.FreshTopN)},
		{"SITEDEX_SEARCH_FRESH_TIMEOUT_MS", intSetter(&cfg.Search.FreshTimeoutMS)},
		{"SITEDEX_SEARCH_AUTO_INDEX_ON_COLD_QUERY", boolSetter(&cfg.Search.AutoIndexOnColdQuery)},

		{"SITEDEX_CHUNKING_TARGET_CHARS", intSetter(&cfg.Chunking.TargetChars)},
		{"SITEDEX_CHUNKING_OVERLAP_CHARS", intSetter(&cfg.Chunking.OverlapChars)},

		{"SITEDEX_LLM_EXTRACTOR_PROVIDER", func(v string) error { cfg.LLMExtractor.Provider = v; return nil }},
		{"SITEDEX_LLM_EXTRACTOR_MODEL", func(v string) error { cfg.LLMExtractor.Model = v; return nil }},
		{"SITEDEX_LLM_EXTRACTOR_API_KEY_ENV", func(v string) error { cfg.LLMExtractor.APIKeyEnv = v; return nil }},
	}

	for _, s := range specs {
		v, ok := os.LookupEnv(s.key)
		if !ok {
			continue
		}
		if err := s.set(v); err != nil {
			return fmt.Errorf("%s=%q: %w", s.key, v, err)
		}
	}
	return nil
}

func floatSetter(dst *float64) func(string) error {
	return func(v string) error {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return err
		}
		*dst = f
		return nil
	}
}

func intSetter(dst *int) func(string) error {
	return func(v string) error {
		i, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		*dst = i
		return nil
	}
}

func boolSetter(dst *bool) func(string) error {
	return func(v string) error {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return err
		}
		*dst = b
		return nil
	}
}

func durationSetter(dst *Duration) func(string) error {
	return func(v string) error {
		d, err := time.ParseDuration(v)
		if err != nil {
			return err
		}
		*dst = Duration(d)
		return nil
	}
}
