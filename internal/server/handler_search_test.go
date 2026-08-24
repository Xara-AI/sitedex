package server

import "testing"

func TestShouldAutoIndex(t *testing.T) {
	cases := []struct {
		source  string
		enabled bool
		want    bool
	}{
		{"site-search", true, true},
		{"site-search", false, false},
		{"index", true, false},
		{"index+live", true, false},
		{"", true, false},
	}
	for _, tc := range cases {
		if got := shouldAutoIndex(tc.source, tc.enabled); got != tc.want {
			t.Errorf("shouldAutoIndex(%q, %v) = %v, want %v", tc.source, tc.enabled, got, tc.want)
		}
	}
}
