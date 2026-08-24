package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/Xara-AI/sitedex/internal/config"
	"github.com/Xara-AI/sitedex/internal/export"
)

func runExport(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	site := fs.String("site", "", "indexed site to export, e.g. example.com (required)")
	format := fs.String("format", "md", "export format: md|jsonl")
	out := fs.String("out", "./kb/", "output directory")
	configPath := fs.String("config", "", "path to sitedex.yaml (optional)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "Usage: sitedex export --site example.com --format md|jsonl --out ./kb/ [--config sitedex.yaml]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *site == "" {
		fs.Usage()
		return fmt.Errorf("--site is required")
	}
	if *format != "md" && *format != "jsonl" {
		fs.Usage()
		return fmt.Errorf("--format must be md or jsonl, got %q", *format)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	if *format == "jsonl" {
		// TODO(M3): JSONL export needs the SQLite index (chunks/products
		// tables) as its source; markdown export below doesn't depend on
		// it since the crawler writes kb/*.md directly.
		_, _ = fmt.Fprintf(stdout, "export: --format jsonl not implemented yet (target milestone M3); site=%s data_dir=%s\n", *site, cfg.DataDir)
		return nil
	}

	n, err := export.CopyMarkdownKB(cfg.DataDir, *site, *out)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "export complete: site=%s format=md out=%s files=%d\n", *site, *out, n)
	return nil
}
