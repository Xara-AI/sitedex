package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/Xara-AI/sitedex/internal/config"
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

	// TODO(M2/M3): implement markdown/JSONL emitters in internal/export,
	// reading from the site's index in cfg.DataDir.
	_, _ = fmt.Fprintf(stdout, "export: not implemented yet (target milestone M2); site=%s format=%s out=%s data_dir=%s\n", *site, *format, *out, cfg.DataDir)
	return nil
}
