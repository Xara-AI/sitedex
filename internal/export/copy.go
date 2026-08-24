package export

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyMarkdownKB copies every file under <dataDir>/<site>/kb/ into outDir,
// preserving the relative directory structure. This is what "sitedex
// export --format md" does: the crawler already wrote clean markdown into
// kb/ as it went (see WritePage), so exporting to a chosen location is
// just a directory copy — "backup = copy the dir" applies here too.
// Returns the number of files copied.
func CopyMarkdownKB(dataDir, site, outDir string) (int, error) {
	kbDir := filepath.Join(dataDir, site, "kb")

	info, err := os.Stat(kbDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("no crawled knowledge base found for %q (looked in %s) — run `sitedex crawl` first", site, kbDir)
		}
		return 0, err
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("%s is not a directory", kbDir)
	}

	count := 0
	err = filepath.Walk(kbDir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(kbDir, p)
		if err != nil {
			return err
		}
		dst := filepath.Join(outDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := copyFile(p, dst); err != nil {
			return err
		}
		count++
		return nil
	})
	if err != nil {
		return count, err
	}
	return count, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
