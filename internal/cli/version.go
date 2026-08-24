package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/Xara-AI/sitedex/internal/version"
)

func runVersion(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, "Usage: sitedex version\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, version.String())
	return nil
}
