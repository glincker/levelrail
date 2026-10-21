package serverready

import "context"

// HostTCPHolder names the local process listening on a TCP port, if ss can see it.
func HostTCPHolder(ctx context.Context, port int) (Listener, bool) {
	out, err := run(ctx, "ss", "-H", "-ltnp")
	if err != nil {
		return Listener{}, false
	}
	for _, l := range ParseSS(out) {
		if l.Port == port {
			return l, true
		}
	}
	return Listener{}, false
}
