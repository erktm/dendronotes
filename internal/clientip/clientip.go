// Package clientip determines the real client address and scheme of a
// request. Forwarding headers (X-Forwarded-For, X-Forwarded-Proto) are only
// believed when the direct peer is a configured trusted proxy.
package clientip

import (
	"net/http"
	"net/netip"
	"strings"
)

// Resolver knows which peers are trusted reverse proxies.
type Resolver struct {
	trusted []netip.Prefix
}

// New returns a resolver that trusts the given networks.
func New(trusted []netip.Prefix) *Resolver {
	return &Resolver{trusted: trusted}
}

// HasTrusted reports whether any trusted proxy is configured.
func (r *Resolver) HasTrusted() bool { return len(r.trusted) > 0 }

// Trusted reports whether addr belongs to a trusted proxy network.
func (r *Resolver) Trusted(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, p := range r.trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// Peer returns the address of the direct TCP peer.
func Peer(req *http.Request) netip.Addr {
	ap, err := netip.ParseAddrPort(req.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	return ap.Addr().Unmap()
}

// FromTrustedProxy reports whether the direct peer is a trusted proxy.
func (r *Resolver) FromTrustedProxy(req *http.Request) bool {
	return r.Trusted(Peer(req))
}

// ClientIP returns the client address. Behind trusted proxies it walks
// X-Forwarded-For from right to left and returns the first address that is
// not itself a trusted proxy, so a client cannot spoof it by sending its own
// header.
func (r *Resolver) ClientIP(req *http.Request) netip.Addr {
	peer := Peer(req)
	if !r.Trusted(peer) {
		return peer
	}
	var hops []string
	for _, v := range req.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(v, ",")...)
	}
	last := peer
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return last
		}
		addr = addr.Unmap()
		if !r.Trusted(addr) {
			return addr
		}
		last = addr
	}
	return last
}

// IsHTTPS reports whether the client connected via HTTPS, either directly or
// to a trusted TLS-terminating proxy.
func (r *Resolver) IsHTTPS(req *http.Request) bool {
	if req.TLS != nil {
		return true
	}
	if !r.FromTrustedProxy(req) {
		return false
	}
	proto, _, _ := strings.Cut(req.Header.Get("X-Forwarded-Proto"), ",")
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

// ParseList parses a comma-separated list of IP addresses and CIDR networks.
func ParseList(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for item := range strings.SplitSeq(s, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			p, err := netip.ParsePrefix(item)
			if err != nil {
				return nil, err
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(item)
		if err != nil {
			return nil, err
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}
