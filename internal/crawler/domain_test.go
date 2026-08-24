package crawler

import "testing"

func TestRegistrableDomain(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{"example.com", "example.com"},
		{"www.example.com", "example.com"},
		{"shop.example.co.uk", "example.co.uk"},
		{"EXAMPLE.com", "example.com"},
		{"example.com:8443", "example.com"},
		{"127.0.0.1:8080", "127.0.0.1"},
		{"localhost:8080", "localhost"},
		{"localhost", "localhost"},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			if got := RegistrableDomain(tc.host); got != tc.want {
				t.Errorf("RegistrableDomain(%q) = %q, want %q", tc.host, got, tc.want)
			}
		})
	}
}

func TestSameRegistrableDomain(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"example.com", "www.example.com", true},
		{"shop.example.com", "blog.example.com", true},
		{"example.com", "example.org", false},
		{"example.co.uk", "other.co.uk", false},
		{"127.0.0.1:8080", "127.0.0.1:9090", true},
	}
	for _, tc := range cases {
		if got := SameRegistrableDomain(tc.a, tc.b); got != tc.want {
			t.Errorf("SameRegistrableDomain(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
