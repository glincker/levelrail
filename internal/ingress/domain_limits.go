package ingress

import "time"

// applyUpstreamTimeouts sets the reverse proxy's read and write timeouts. Both
// zero returns handle untouched so routes without domain limits render as before.
func applyUpstreamTimeouts(handle []any, read, write time.Duration) []any {
	if (read <= 0 && write <= 0) || len(handle) == 0 {
		return handle
	}
	rp, ok := handle[len(handle)-1].(ReverseProxyHandler)
	if !ok {
		return handle
	}
	t := HTTPTransport{Protocol: "http"}
	if rp.Transport != nil {
		t = *rp.Transport
	}
	if read > 0 {
		t.ReadTimeout = read.String()
	}
	if write > 0 {
		t.WriteTimeout = write.String()
	}
	rp.Transport = &t
	out := append([]any(nil), handle...)
	out[len(out)-1] = rp
	return out
}
