package serverready

import (
	"bufio"
	"context"
	"strconv"
	"strings"
)

// ParseSSUDP reads `ss -H -lunp` output into bound UDP sockets.
func ParseSSUDP(out string) []Listener {
	var ls []Listener
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 5 || !strings.EqualFold(f[0], "UNCONN") {
			continue
		}
		addr, port, ok := splitHostPort(f[3])
		if !ok {
			continue
		}
		l := Listener{Addr: addr, Port: port}
		if m := ssUsersPattern.FindStringSubmatch(sc.Text()); m != nil {
			l.Process = m[1]
			l.PID, _ = strconv.Atoi(m[2])
		}
		ls = append(ls, l)
	}
	return ls
}

// HostUDPHolder names the local process bound to a UDP port, if ss can see it.
func HostUDPHolder(ctx context.Context, port int) (Listener, bool) {
	out, err := run(ctx, "ss", "-H", "-lunp")
	if err != nil {
		return Listener{}, false
	}
	for _, l := range ParseSSUDP(out) {
		if l.Port == port {
			return l, true
		}
	}
	return Listener{}, false
}
