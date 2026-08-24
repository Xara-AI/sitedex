// Command sitedex crawls a website into a clean, chunked, RAG-ready
// knowledge base and serves fast product/content search over it.
package main

import (
	"os"

	"github.com/Xara-AI/sitedex/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
