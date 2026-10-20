package diagnose

import (
	"sort"
	"strconv"
	"strings"
)

const tcpListenState = "0A"

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
