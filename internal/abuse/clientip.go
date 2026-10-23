package abuse

import (
	"net"
	"net/netip"
	"strings"
)

// ParseTrusted parses a comma or space separated list of IPs and CIDRs into
// the proxy ranges forwarded headers are believed from.
func ParseTrusted(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, f := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
		if strings.EqualFold(f, "private_ranges") {
			out = append(out, PrivateRanges()...)
			continue
		}
		p, err := ParseDenyEntry(f)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// PrivateRanges are the loopback, link-local and RFC 1918 / ULA ranges.
func PrivateRanges() []netip.Prefix {
	return []netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("::1/128"),
		netip.MustParsePrefix("fc00::/7"), netip.MustParsePrefix("fe80::/10"),
	}
}

func inRanges(a netip.Addr, ranges []netip.Prefix) bool {
	for _, p := range ranges {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// ResolveClientIP picks the client address for a request. Forwarded headers
// are believed only when the TCP peer is itself a trusted proxy, and then the
// chain is walked from the right, skipping trusted hops, so a value the client
// put at the front of X-Forwarded-For is never reached. Without a trusted
// peer the peer address is the client, whatever the headers say.
func ResolveClientIP(remoteAddr string, forwardedFor []string, trusted []netip.Prefix) netip.Addr {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	peer = peer.Unmap()
	if len(trusted) == 0 || len(forwardedFor) == 0 || !inRanges(peer, trusted) {
		return peer
	}
	client := peer
	for i := len(forwardedFor) - 1; i >= 0; i-- {
		list := forwardedFor[i]
		for list != "" {
			var hop string
			if j := strings.LastIndexByte(list, ','); j >= 0 {
				hop, list = list[j+1:], list[:j]
			} else {
				hop, list = list, ""
			}
			a, err := netip.ParseAddr(strings.TrimSpace(hop))
			if err != nil {
				return client
			}
			client = a.Unmap()
			if !inRanges(client, trusted) {
				return client
			}
		}
	}
	return client
}
