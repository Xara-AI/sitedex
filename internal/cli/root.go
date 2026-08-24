// Package cli implements the sitedex command-line surface: crawl, export,
// search, serve, sites, version. See CLAUDE.md for the full CLI/API spec.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// subcommand runs one sitedex subcommand against the given args, writing
// normal output to stdout and errors/usage to stderr.
type subcommand func(args []string, stdout, stderr io.Writer) error

// Run parses os.Args-style args (excluding the program name), dispatches to
// the requested subcommand, and returns the process exit code.
func Run(args []string) int {
	return RunWithIO(args, os.Stdout, os.Stderr)
}

// RunWithIO is Run with injectable stdout/stderr, for testing.
func RunWithIO(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	name, rest := args[0], args[1:]

	if name == "-h" || name == "--help" || name == "help" {
		printUsage(stdout)
		return 0
	}

	cmds := map[string]subcommand{
		"crawl":   runCrawl,
		"export":  runExport,
		"search":  runSearch,
		"serve":   runServe,
		"sites":   runSites,
		"version": runVersion,
	}

	cmd, ok := cmds[name]
	if !ok {
		_, _ = fmt.Fprintf(stderr, "sitedex: unknown command %q\n\n", name)
		printUsage(stderr)
		return 2
	}

	if err := cmd(rest, stdout, stderr); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		_, _ = fmt.Fprintf(stderr, "sitedex %s: %v\n", name, err)
		return 1
	}
	return 0
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `sitedex — crawl a website into a clean, chunked, RAG-ready knowledge base
and serve fast product/content search over it.

Usage:
  sitedex <command> [flags]

Commands:
  crawl    Crawl a site into markdown + a searchable index
  export   Dump an indexed site's knowledge base as markdown or JSONL
  search   Query an indexed site
  serve    Run the long-lived HTTP API daemon
  sites    List indexed sites and their stats
  version  Print the sitedex version

Run "sitedex <command> -h" for flags on a specific command.
`)
}
