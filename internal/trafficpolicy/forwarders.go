package trafficpolicy

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Forwarder actions.
const (
	ActionApp      = "app"
	ActionURL      = "url"
	ActionRedirect = "redirect"
)

// Host header modes for a forwarder.
const (
	HostPreserve = "preserve"
	HostUpstream = "upstream"
	HostCustom   = "custom"
)

// Forwarder is one ordered path rule. The first matching rule wins; a
// request no rule matches goes to the domain's own app.
type Forwarder struct {
	Name           string `json:"name,omitempty"`
	Match          Match  `json:"match"`
	Action         string `json:"action"`
	App            string `json:"app,omitempty"`
	URL            string `json:"url,omitempty"`
	RedirectStatus int    `json:"redirect_status,omitempty"`
	// StripPrefix removes Match.Path before forwarding (prefix rules only).
	StripPrefix bool `json:"strip_prefix,omitempty"`
	// RewritePrefix is prepended to the (possibly stripped) path.
	RewritePrefix string `json:"rewrite_prefix,omitempty"`
	Host          string `json:"host,omitempty"`
	HostValue     string `json:"host_value,omitempty"`
	// WebSocket nil or true passes Upgrade requests through.
	WebSocket      *bool `json:"websocket,omitempty"`
	TimeoutSeconds int   `json:"timeout_seconds,omitempty"`
}

// Forwarders is a domain's ordered path rules.
type Forwarders struct {
	Rules []Forwarder `json:"rules"`
}

// WebSocketEnabled reports whether Upgrade requests pass through.
func (f Forwarder) WebSocketEnabled() bool { return f.WebSocket == nil || *f.WebSocket }

// HostMode returns the effective Host header mode: external URLs default to
// the upstream's own host, apps keep the visitor's.
func (f Forwarder) HostMode() string {
	if f.Host != "" {
		return f.Host
	}
	if f.Action == ActionURL {
		return HostUpstream
	}
	return HostPreserve
}

var redirectStatuses = map[int]bool{301: true, 302: true, 307: true, 308: true}

// Validate checks every rule. External URL targets get a structural check
// here; the address check (SSRF guard) needs DNS and runs in CheckTarget.
func (fw *Forwarders) Validate(l Limits, c Context) error {
	col := &collector{kind: KindForwarders}
	if len(fw.Rules) > l.MaxForwarders {
		col.add("rules", "at most %d forwarders per domain", l.MaxForwarders)
	}
	for i, f := range fw.Rules {
		field := fmt.Sprintf("rules[%d]", i)
		validateMatch(col, field+".match", f.Match, l, nil)
		if len(f.Name) > 80 || !SafeHeaderValue(f.Name) {
			col.add(field+".name", "must be a single line of at most 80 characters")
		}
		switch f.Action {
		case ActionApp:
			if f.App == "" {
				col.add(field+".app", "is required")
			} else if c.AppExists != nil && !c.AppExists(f.App) {
				col.add(field+".app", "app %q does not exist", f.App)
			}
		case ActionURL:
			if err := validateExternalURL(f.URL, c.Domain); err != "" {
				col.add(field+".url", "%s", err)
			}
		case ActionRedirect:
			if f.RedirectStatus != 0 && !redirectStatuses[f.RedirectStatus] {
				col.add(field+".redirect_status", "must be 301, 302, 307 or 308")
			}
			if loop := redirectLoop(f, c.Domain); loop != "" {
				col.add(field+".url", "%s", loop)
			}
		default:
			col.add(field+".action", "must be app, url or redirect")
		}
		if f.StripPrefix && f.Match.Kind != MatchPrefix {
			col.add(field+".strip_prefix", "only applies to prefix matches")
		}
		if f.RewritePrefix != "" && (!strings.HasPrefix(f.RewritePrefix, "/") || strings.ContainsAny(f.RewritePrefix, "?#{} \t") || len(f.RewritePrefix) > 256) {
			col.add(field+".rewrite_prefix", "must be a plain path starting with /")
		}
		switch f.HostMode() {
		case HostPreserve, HostUpstream:
		case HostCustom:
			if !validHostname(f.HostValue) {
				col.add(field+".host_value", "must be a hostname")
			}
		default:
			col.add(field+".host", "must be preserve, upstream or custom")
		}
		maxTimeout := int(l.ForwardMaxTimeout.Seconds())
		if f.TimeoutSeconds < 0 || f.TimeoutSeconds > maxTimeout {
			col.add(field+".timeout_seconds", "must be between 0 (default) and %d", maxTimeout)
		}
	}
	return col.err()
}

func validateExternalURL(raw, domain string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "must be an absolute http(s) URL, e.g. https://api.example.com"
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "must not include credentials, a query or a fragment"
	}
	if strings.ContainsAny(raw, "{} \t") {
		return "must not contain braces or spaces"
	}
	if strings.EqualFold(u.Hostname(), domain) {
		return "forwards back to this same domain, which would loop"
	}
	if _, _, err := net.SplitHostPort(u.Host); err == nil && u.Port() == "" {
		return "has an empty port"
	}
	return ""
}

// redirectLoop reports a redirect that sends a matching request to a URL
// the same rule matches again on the same host.
func redirectLoop(f Forwarder, domain string) string {
	u, err := url.Parse(f.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "must be an absolute http(s) URL"
	}
	if strings.ContainsAny(f.URL, " \t") || !SafeHeaderValue(f.URL) {
		return "must not contain spaces or control characters"
	}
	if strings.EqualFold(u.Hostname(), domain) {
		path := u.Path
		if path == "" {
			path = "/"
		}
		if f.Match.Matches("GET", path) || (len(f.Match.Methods) > 0 && f.Match.Matches(f.Match.Methods[0], path)) {
			return "redirects to a path this same rule matches, which would loop"
		}
	}
	return ""
}

func validHostname(h string) bool {
	if h == "" || len(h) > 253 {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	return true
}
