package crawler

import (
	"net"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// RegistrableDomain returns the registrable domain (eTLD+1) for host, e.g.
// "shop.example.co.uk" -> "example.co.uk". If host has no recognized
// public suffix (bare hostnames, IP addresses, "localhost", or
// "host:port" pairs used in tests), it falls back to returning host
// unchanged (lowercased, port stripped) so local/intranet crawling still
// works predictably.
func RegistrableDomain(host string) string {
	host = strings.ToLower(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if dom, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
		return dom
	}
	return host
}

// SameRegistrableDomain reports whether hostA and hostB share the same
// registrable domain.
func SameRegistrableDomain(hostA, hostB string) bool {
	return RegistrableDomain(hostA) == RegistrableDomain(hostB)
}
