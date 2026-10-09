package ingress

// WakePath is the control plane endpoint a wake route proxies to.
const WakePath = "/api/v1/hooks/wake"

// Headers the wake route sends so the control plane knows which app to start.
const (
	WakeTokenHeader = "X-Wake-Token" //nolint:gosec // header name, not a credential
	WakeAppHeader   = "X-Wake-App"
	WakeURIHeader   = "X-Wake-Uri"
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

// RewriteHandler is Caddy's "rewrite" handler: it replaces the request URI
// and, when set, the method. The wake hook is GET-only whatever the client sent.
type RewriteHandler struct {
	Handler string `json:"handler"`
	URI     string `json:"uri"`
	Method  string `json:"method,omitempty"`
}

func (r WakeRoute) handlers(h Hardening) []any {
	page := h.UnavailableResponse("Waking up", "This app was asleep and is starting now. This page will retry on its own.")
	proxy := NewReverseProxyHandler(r.Dial)
	proxy.Headers = &ProxyHeaders{Request: &HeaderOps{Set: map[string][]string{
		WakeTokenHeader: {r.Token},
		WakeAppHeader:   {r.App},
		// Caddy fills this per request; the rewrite above replaced the path.
		WakeURIHeader: {"{http.request.orig_uri}"},
	}}}
	// Only the 204 (not asleep or no hold) becomes the retrying page; the hook's
	// 307 replay must reach the client untouched.
	proxy.HandleResponse = []ResponseHandler{{Match: &ResponseMatcher{StatusCode: []int{204}}, Routes: []Route{{Handle: []any{page}}}}}
	return []any{RewriteHandler{Handler: "rewrite", URI: WakePath, Method: "GET"}, proxy}
}
