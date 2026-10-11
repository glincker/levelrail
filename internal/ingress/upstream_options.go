package ingress

import (
	"strings"
	"time"
)

// UpstreamHostPlaceholder sends the dialed upstream's host:port as Host.
const UpstreamHostPlaceholder = "{http.reverse_proxy.upstream.hostport}"

// UpstreamOptions shape one proxy route's single backend: TLS toward it,
// the Host it sees, timeouts, websockets and path rewriting.
type UpstreamOptions struct {
	TLS                bool
	InsecureSkipVerify bool
	ServerName         string
	// HostHeader is a literal Host value, escaped before use; empty keeps
	// the client's Host. HostFromUpstream wins over it.
	HostHeader            string
	HostFromUpstream      bool
	DisableWebSocket      bool
	DialTimeout           time.Duration
	ResponseHeaderTimeout time.Duration
	StripPrefix           string
	// BasePath is prepended to every forwarded path (a URL target's path).
	BasePath string
}

// apply merges u into the reverse proxy that ends handle and puts the path
// rewrites right in front of it.
func (u *UpstreamOptions) apply(handle []any) []any {
	if len(handle) == 0 {
		return handle
	}
	rp, ok := handle[len(handle)-1].(ReverseProxyHandler)
	if !ok {
		return handle
	}
	u.applyTransport(&rp)
	u.applyHeaders(&rp)
	var pre []any
	if prefix := strings.TrimSuffix(u.StripPrefix, "/"); prefix != "" {
		pre = append(pre, rewriteHandler{Handler: "rewrite", StripPathPrefix: escapePlaceholders(prefix)})
	}
	if base := strings.TrimSuffix(u.BasePath, "/"); base != "" {
		pre = append(pre, rewriteHandler{Handler: "rewrite", URI: escapePlaceholders(base) + "{http.request.uri}"})
	}
	out := make([]any, 0, len(handle)+len(pre))
	out = append(out, handle[:len(handle)-1]...)
	out = append(out, pre...)
	return append(out, rp)
}

func (u *UpstreamOptions) applyTransport(rp *ReverseProxyHandler) {
	if !u.TLS && u.DialTimeout <= 0 && u.ResponseHeaderTimeout <= 0 {
		return
	}
	t := HTTPTransport{Protocol: "http"}
	if rp.Transport != nil {
		t = *rp.Transport
	}
	if u.DialTimeout > 0 {
		t.DialTimeout = u.DialTimeout.String()
	}
	if u.ResponseHeaderTimeout > 0 {
		t.ResponseHeaderTimeout = u.ResponseHeaderTimeout.String()
	}
	if u.TLS {
		t.TLS = &UpstreamTLSConfig{InsecureSkipVerify: u.InsecureSkipVerify, ServerName: u.ServerName}
	}
	rp.Transport = &t
}

func (u *UpstreamOptions) applyHeaders(rp *ReverseProxyHandler) {
	host := ""
	switch {
	case u.HostFromUpstream:
		host = UpstreamHostPlaceholder
	case u.HostHeader != "":
		host = escapePlaceholders(u.HostHeader)
	}
	if host == "" && !u.DisableWebSocket {
		return
	}
	ops := HeaderOps{}
	if rp.Headers != nil && rp.Headers.Request != nil {
		ops = *rp.Headers.Request
	}
	set := make(map[string][]string, len(ops.Set)+1)
	for k, v := range ops.Set {
		set[k] = v
	}
	if host != "" {
		set["Host"] = []string{host}
	}
	ops.Set = set
	if u.DisableWebSocket {
		ops.Delete = append(append([]string(nil), ops.Delete...), "Upgrade")
	}
	rp.Headers = &ProxyHeaders{Request: &ops}
}
