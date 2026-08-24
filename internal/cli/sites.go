package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/Xara-AI/sitedex/internal/config"
)

func runSites(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("sites", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "", "path to sitedex.yaml (optional)")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "Usage: sitedex sites [--config sitedex.yaml]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	// TODO(M3): list <site>/index.db entries under cfg.DataDir with doc
	// counts and last-crawl timestamps.
	_, _ = fmt.Fprintf(stdout, "sites: not implemented yet (target milestone M3); data_dir=%s\n", cfg.DataDir)
	return nil
}
