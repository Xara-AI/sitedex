package cli

import (
	"bytes"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = RunWithIO(args, &out, &errOut)
	return code, out.String(), errOut.String()
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
	code, stdout, _ := run(t, "crawl", "--site", "https://example.com")
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "site=https://example.com") {
		t.Errorf("stdout = %q, want it to echo the site flag", stdout)
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
