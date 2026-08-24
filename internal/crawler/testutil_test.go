package crawler

import (
	"os"
	"testing"
	"time"
)

// testTimeout returns a channel that fires after a generous bound, for
// tests asserting that some operation terminates rather than hanging.
func testTimeout(t *testing.T) <-chan time.Time {
	t.Helper()
	return time.After(5 * time.Second)
}

// writeFileForTest overwrites path with contents, failing the test on
// error.
func writeFileForTest(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
