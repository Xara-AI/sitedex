package cli

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = RunWithIO(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// writeTestConfig writes a minimal sitedex.yaml pointing data_dir at dir
// (so crawl/export tests never touch the real ./sitedex-data default) with
// a fast rate limit, and returns its path.
func writeTestConfig(t *testing.T, dataDir string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sitedex.yaml")
	yaml := "data_dir: " + dataDir + "\ncrawl:\n  rate_limit_rps: 1000\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	return path
}

func TestRun_NoArgsPrintsUsage(t *testing.T) {
	code, _, stderr := run(t)
	if code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	code, _, stderr := run(t, "bogus")
	if code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
	if !strings.Contains(stderr, `unknown command "bogus"`) {
		t.Errorf("stderr = %q, want unknown command message", stderr)
	}
}

func TestRun_Help(t *testing.T) {
	code, stdout, _ := run(t, "--help")
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_Version(t *testing.T) {
	code, stdout, _ := run(t, "version")
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "sitedex") {
		t.Errorf("stdout = %q, want it to mention sitedex", stdout)
	}
}

func TestRun_CrawlRequiresSite(t *testing.T) {
	code, _, stderr := run(t, "crawl")
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--site is required") {
		t.Errorf("stderr = %q, want missing --site error", stderr)
	}
}

func TestRun_CrawlWithSite(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><main><p>Home page content long enough to clear the density threshold nicely for this CLI test.</p></main></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	configPath := writeTestConfig(t, t.TempDir())

	code, stdout, stderr := run(t, "crawl", "--site", srv.URL+"/", "--config", configPath)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "fetched=1") {
		t.Errorf("stdout = %q, want fetched=1", stdout)
	}
}

func TestRun_CrawlThenExportMD(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><main><p>Home page content long enough to clear the density threshold nicely for this CLI test.</p></main></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dataDir := t.TempDir()
	configPath := writeTestConfig(t, dataDir)

	if code, _, stderr := run(t, "crawl", "--site", srv.URL+"/", "--config", configPath); code != 0 {
		t.Fatalf("crawl code = %d, want 0 (stderr: %s)", code, stderr)
	}

	// The crawler keys each site's directory by RegistrableDomain(seed.Host),
	// which for a bare IP:port falls back to the IP with the port stripped
	// (see internal/crawler/domain.go) — match that here.
	site, _, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	outDir := t.TempDir()
	code, stdout, stderr := run(t, "export", "--site", site, "--format", "md", "--out", outDir, "--config", configPath)
	if code != 0 {
		t.Fatalf("export code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "files=1") {
		t.Errorf("stdout = %q, want files=1", stdout)
	}
	if _, err := os.Stat(filepath.Join(outDir, "index.md")); err != nil {
		t.Errorf("expected exported index.md: %v", err)
	}
}

func TestRun_SearchRequiresSiteAndQuery(t *testing.T) {
	code, _, stderr := run(t, "search", "--site", "example.com")
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--site and --query are required") {
		t.Errorf("stderr = %q, want missing flags error", stderr)
	}
}

func TestRun_ExportRejectsBadFormat(t *testing.T) {
	code, _, stderr := run(t, "export", "--site", "example.com", "--format", "xml")
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--format must be md or jsonl") {
		t.Errorf("stderr = %q, want format error", stderr)
	}
}

func TestRun_ServeDefaultAddr(t *testing.T) {
	code, stdout, _ := run(t, "serve")
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "listen=:8080") {
		t.Errorf("stdout = %q, want default listen addr", stdout)
	}
}

func TestRun_SitesRuns(t *testing.T) {
	code, stdout, _ := run(t, "sites")
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "sitedex-data") {
		t.Errorf("stdout = %q, want default data_dir", stdout)
	}
}
