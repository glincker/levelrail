package ingress

import "time"

// DomainTuning is a domain's own proxy tuning. Every field is a flag or a
// bounded duration checked by the API, so no tenant text reaches the config.
type DomainTuning struct {
	Compression           bool
	Streaming             bool
	DialTimeout           time.Duration
	ResponseHeaderTimeout time.Duration
	// StreamTimeout and StreamCloseDelay shape upgraded (websocket) and
	// streaming connections; they never touch matchers or auth handlers.
	StreamTimeout    time.Duration
	StreamCloseDelay time.Duration
}

// flushImmediately is reverse_proxy's flush_interval value for "write each chunk now".
const flushImmediately = -1

// applyDomainTuning rewrites only the trailing reverse proxy handler and, for
// compression, puts an encode handler right before it. Nil returns handle as is.
func applyDomainTuning(handle []any, t *DomainTuning) []any {
	if t == nil || len(handle) == 0 {
		return handle
	}
	rp, ok := handle[len(handle)-1].(ReverseProxyHandler)
	if !ok {
		return handle
	}
	if t.DialTimeout > 0 || t.ResponseHeaderTimeout > 0 {
		tr := HTTPTransport{Protocol: "http"}
		if rp.Transport != nil {
			tr = *rp.Transport
		}
		if t.DialTimeout > 0 {
			tr.DialTimeout = t.DialTimeout.String()
		}
		if t.ResponseHeaderTimeout > 0 {
			tr.ResponseHeaderTimeout = t.ResponseHeaderTimeout.String()
		}
		rp.Transport = &tr
	}
	if t.Streaming {
		rp.FlushInterval = flushImmediately
	}
	if t.StreamTimeout > 0 {
		rp.StreamTimeout = t.StreamTimeout.String()
	}
	if t.StreamCloseDelay > 0 {
		rp.StreamCloseDelay = t.StreamCloseDelay.String()
	}
	out := append([]any(nil), handle[:len(handle)-1]...)
	if t.Compression {
		out = append(out, NewStaticEncodeHandler())
	}
	return append(out, rp)
}
