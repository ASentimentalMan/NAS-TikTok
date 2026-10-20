package access

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIPDoesNotTrustUnconfiguredForwardedHeader(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "192.0.2.10:443"
	request.Header.Set("X-Forwarded-For", "198.51.100.7")

	address, err := ClientIP(request, nil)
	if err != nil || address.String() != "192.0.2.10" {
		t.Fatalf("ClientIP() = %v, %v; want direct peer 192.0.2.10", address, err)
	}
}

func TestClientIPWalksTrustedProxyChainFromRight(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "10.0.0.2:443"
	request.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.1")
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}

	address, err := ClientIP(request, trusted)
	if err != nil || address.String() != "198.51.100.7" {
		t.Fatalf("ClientIP() = %v, %v; want 198.51.100.7", address, err)
	}
}

func TestAllowedIPAddressesAndCIDRBoundaries(t *testing.T) {
	allowlist := []netip.Prefix{
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("2001:db8::/32"),
	}
	for _, address := range []string{"192.0.2.0", "192.0.2.255", "2001:db8::1"} {
		if !Allowed(netip.MustParseAddr(address), allowlist) {
			t.Errorf("Allowed(%s) = false", address)
		}
	}
	for _, address := range []string{"192.0.3.1", "2001:db9::1", "127.0.0.9", "::1"} {
		if Allowed(netip.MustParseAddr(address), allowlist) {
			t.Errorf("Allowed(%s) = true", address)
		}
	}
}

func TestClientIPRejectsInvalidAddressesAndMappedIPv4MatchesIPv4CIDR(t *testing.T) {
	for _, remote := range []string{"not-an-address:1234", "999.0.0.1:1234", "2001:db8:::1"} {
		request := httptest.NewRequest("GET", "/", nil)
		request.RemoteAddr = remote
		if address, err := ClientIP(request, nil); err == nil {
			t.Errorf("ClientIP(%q) = %v, want invalid address", remote, address)
		}
	}

	allowlist := []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}
	mapped := netip.MustParseAddr("::ffff:192.0.2.8")
	if !Allowed(mapped, allowlist) {
		t.Fatal("IPv4-mapped IPv6 address did not match its IPv4 CIDR")
	}
}

func TestClientIPRejectsMalformedForwardedChainOnlyFromTrustedProxy(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "10.0.0.2:443"
	request.Header.Set("X-Forwarded-For", "198.51.100.7, invalid")
	if address, err := ClientIP(request, trusted); err == nil {
		t.Fatalf("trusted proxy malformed chain resolved as %v", address)
	}

	request.RemoteAddr = "192.0.2.10:443"
	if address, err := ClientIP(request, trusted); err != nil || address.String() != "192.0.2.10" {
		t.Fatalf("untrusted proxy header changed peer address: %v, %v", address, err)
	}
}

func TestLoopbackRequiresExplicitAllowlistWhenOneIsConfigured(t *testing.T) {
	loopback := netip.MustParseAddr("127.0.0.1")
	if !Allowed(loopback, nil) {
		t.Fatal("loopback was rejected when no allowlist is configured")
	}
	if Allowed(loopback, []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}) {
		t.Fatal("loopback bypassed a configured allowlist")
	}
	if !Allowed(loopback, []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}) {
		t.Fatal("explicitly allowlisted loopback was rejected")
	}
}
