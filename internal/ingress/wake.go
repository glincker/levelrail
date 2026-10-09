package ingress

// WakePath is the control plane endpoint a wake route proxies to.
const WakePath = "/api/v1/hooks/wake"

// Headers the wake route sends so the control plane knows which app to start.
const (
	WakeTokenHeader = "X-Wake-Token" //nolint:gosec // header name, not a credential
	WakeAppHeader   = "X-Wake-App"
)

// WakeRoute is a sleeping app's domains: any request starts the app and gets
// the friendly retrying page instead of an error.
type WakeRoute struct {
	Hosts []string
	// Dial is the control plane's own listener, Token its shared wake secret.
	Dial  string
	Token string
	App   string
}

// RewriteHandler is Caddy's "rewrite" handler: it replaces the request URI.
type RewriteHandler struct {
	Handler string `json:"handler"`
	URI     string `json:"uri"`
}

func (r WakeRoute) handlers(h Hardening) []any {
	page := h.UnavailableResponse("Waking up", "This app was asleep and is starting now. This page will retry on its own.")
	proxy := NewReverseProxyHandler(r.Dial)
	proxy.Headers = &ProxyHeaders{Request: &HeaderOps{Set: map[string][]string{
		WakeTokenHeader: {r.Token},
		WakeAppHeader:   {r.App},
	}}}
	proxy.HandleResponse = []ResponseHandler{{Routes: []Route{{Handle: []any{page}}}}}
	return []any{RewriteHandler{Handler: "rewrite", URI: WakePath}, proxy}
}
