package access

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

var ErrInvalidClientIP = errors.New("invalid client IP address")

func ClientIP(request *http.Request, trustedProxies []netip.Prefix) (netip.Addr, error) {
	remote, err := parseRemoteAddr(request.RemoteAddr)
	if err != nil {
		return netip.Addr{}, err
	}
	remote = remote.Unmap()
	if !contains(trustedProxies, remote) {
		return remote, nil
	}

	forwarded := request.Header.Get("X-Forwarded-For")
	if strings.TrimSpace(forwarded) == "" {
		return remote, nil
	}
	hops := strings.Split(forwarded, ",")
	addresses := make([]netip.Addr, 0, len(hops))
	for _, hop := range hops {
		address, parseErr := netip.ParseAddr(strings.TrimSpace(hop))
		if parseErr != nil {
			return netip.Addr{}, ErrInvalidClientIP
		}
		addresses = append(addresses, address.Unmap())
	}

	candidate := remote
	for index := len(addresses) - 1; index >= 0; index-- {
		if !contains(trustedProxies, candidate) {
			return candidate, nil
		}
		candidate = addresses[index]
	}
	return candidate, nil
}

func Allowed(address netip.Addr, allowlist []netip.Prefix) bool {
	address = address.Unmap()
	if len(allowlist) == 0 {
		return true
	}
	return contains(allowlist, address)
}

func IsTrustedProxy(request *http.Request, trustedProxies []netip.Prefix) bool {
	remote, err := parseRemoteAddr(request.RemoteAddr)
	return err == nil && contains(trustedProxies, remote.Unmap())
}

func contains(prefixes []netip.Prefix, address netip.Addr) bool {
	address = address.Unmap()
	for _, prefix := range prefixes {
		prefixAddress := prefix.Addr()
		if prefixAddress.Is4In6() && address.Is4() && prefix.Bits() >= 96 {
			prefix = netip.PrefixFrom(prefixAddress.Unmap(), prefix.Bits()-96).Masked()
		}
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func parseRemoteAddr(value string) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		host = strings.Trim(value, "[]")
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, ErrInvalidClientIP
	}
	return address, nil
}
