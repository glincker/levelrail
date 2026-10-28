package diagnose

import (
	"encoding/hex"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

const (
	tcpListenState = "0A"
	// udpUnconnected is TCP_CLOSE, the state of a bound UDP server socket.
	udpUnconnected = "07"
)

// ParseListeningPorts extracts listening TCP ports from /proc/net/tcp or
// /proc/net/tcp6 content. Ports bound to every address come first, then
// other non-loopback ports, then loopback-only ones, each group ascending.
func ParseListeningPorts(tables ...string) []int {
	type entry struct {
		port int
		rank int
	}
	best := map[int]int{}
	for _, table := range tables {
		for _, ln := range strings.Split(table, "\n") {
			f := strings.Fields(ln)
			if len(f) < 4 || f[3] != tcpListenState {
				continue
			}
			addr, portHex, ok := strings.Cut(f[1], ":")
			if !ok {
				continue
			}
			p, err := strconv.ParseInt(portHex, 16, 32)
			if err != nil || p <= 0 {
				continue
			}
			r := addrRank(addr)
			if cur, seen := best[int(p)]; !seen || r < cur {
				best[int(p)] = r
			}
		}
	}
	entries := make([]entry, 0, len(best))
	for p, r := range best {
		entries = append(entries, entry{p, r})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].rank != entries[j].rank {
			return entries[i].rank < entries[j].rank
		}
		return entries[i].port < entries[j].port
	})
	out := make([]int, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.port)
	}
	return out
}

func addrRank(hexAddr string) int {
	switch {
	case strings.Trim(hexAddr, "0") == "":
		return 0
	case hexAddr == "0100007F" || hexAddr == "00000000000000000000000001000000":
		return 2
	default:
		return 1
	}
}

// BoundSocket is one bound local UDP address and port.
type BoundSocket struct {
	Addr netip.Addr
	Port int
}

// ParseBoundUDP extracts bound, unconnected UDP sockets from /proc/net/udp
// or /proc/net/udp6 content, in table order. Connected client sockets are
// skipped: they hold ephemeral ports, not services.
func ParseBoundUDP(tables ...string) []BoundSocket {
	var out []BoundSocket
	for _, table := range tables {
		for _, ln := range strings.Split(table, "\n") {
			f := strings.Fields(ln)
			if len(f) < 4 || f[3] != udpUnconnected {
				continue
			}
			addrHex, portHex, ok := strings.Cut(f[1], ":")
			if !ok {
				continue
			}
			p, err := strconv.ParseInt(portHex, 16, 32)
			addr, aok := procAddr(addrHex)
			if err != nil || p <= 0 || !aok {
				continue
			}
			out = append(out, BoundSocket{Addr: addr, Port: int(p)})
		}
	}
	return out
}

// procAddr decodes a /proc/net address: 32-bit words in host (little
// endian) byte order, one word for IPv4, four for IPv6.
func procAddr(h string) (netip.Addr, bool) {
	b, err := hex.DecodeString(h)
	if err != nil || (len(b) != 4 && len(b) != 16) {
		return netip.Addr{}, false
	}
	for i := 0; i < len(b); i += 4 {
		b[i], b[i+1], b[i+2], b[i+3] = b[i+3], b[i+2], b[i+1], b[i]
	}
	if len(b) == 4 {
		return netip.AddrFrom4([4]byte(b)), true
	}
	return netip.AddrFrom16([16]byte(b)), true
}
