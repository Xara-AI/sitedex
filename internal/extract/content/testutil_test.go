package content

import "time"

// timeoutCh returns a channel that fires after a generous bound, for tests
// asserting that some operation terminates rather than hanging.
func timeoutCh() <-chan time.Time {
	return time.After(5 * time.Second)
}
