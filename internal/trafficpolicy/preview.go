package trafficpolicy

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Step is one stage of the human readable "what happens to this request"
// preview, in the order the ingress evaluates them.
type Step struct {
	Stage   string `json:"stage"`
	Outcome string `json:"outcome"`
	Final   bool   `json:"final,omitempty"`
}

// Explain walks method and path through p in ingress order.
func Explain(p Policy, method, path string) []Step {
	if method == "" {
		method = "GET"
	}
	if path == "" || path[0] != '/' {
		path = "/" + path
	}
	var steps []Step
	if r := p.Redirects; r != nil {
		if r.LowercaseHost {
			steps = append(steps, Step{Stage: KindRedirects, Outcome: "a host with upper case letters is redirected to its lower case form"})
		}
		switch r.SlashMode() {
		case SlashAdd:
			if !strings.HasSuffix(path, "/") && !strings.Contains(lastSegment(path), ".") {
				return append(steps, Step{Stage: KindRedirects, Outcome: fmt.Sprintf("308 redirect to %s/ (trailing slash added)", path), Final: true})
			}
		case SlashRemove:
			if path != "/" && strings.HasSuffix(path, "/") {
				return append(steps, Step{Stage: KindRedirects, Outcome: fmt.Sprintf("308 redirect to %s (trailing slash removed)", strings.TrimSuffix(path, "/")), Final: true})
			}
		}
	}
	if g := p.Geo; g != nil {
		list := strings.Join(g.Countries, ", ")
		verb := "only visitors from " + list + " are let through"
		if g.Mode == GeoDeny {
			verb = "visitors from " + list + " are stopped"
		}
		steps = append(steps, Step{Stage: KindGeo, Outcome: fmt.Sprintf("%s (%s); private addresses and %d exempt entries always pass", verb, geoActionText(*g), len(g.Exempt))})
	}
	if h := p.Headers; h != nil {
		if h.CORS != nil && h.CORS.Preflight && method == "OPTIONS" {
			return append(steps, Step{Stage: KindHeaders, Outcome: "CORS preflight from an allowed origin is answered with 204 by the ingress", Final: true})
		}
		var req []string
		for _, r := range h.Rules {
			if r.Side == SideRequest {
				req = append(req, ruleText(r))
			}
		}
		if h.ForwardedPrefix != "" {
			req = append(req, "set X-Forwarded-Prefix: "+h.ForwardedPrefix)
		}
		if len(req) > 0 {
			steps = append(steps, Step{Stage: KindHeaders, Outcome: "request headers: " + strings.Join(req, "; ")})
		}
	}
	if c := p.Cache; c != nil && c.Enabled {
		outcome := "not cached (no rule matches)"
		if method != "GET" && method != "HEAD" {
			outcome = "not cached (only GET and HEAD are cached)"
		} else {
			for i, r := range c.Rules {
				if r.Match.Matches(method, path) {
					outcome = fmt.Sprintf("cache rule %d: served from cache when fresh (TTL %ds), skipped when the request has Authorization%s", i+1, r.TTLSeconds, cookieText(r))
					break
				}
			}
		}
		steps = append(steps, Step{Stage: KindCache, Outcome: outcome})
	}
	steps = append(steps, forwardStep(p.Forwarders, method, path))
	if h := p.Headers; h != nil {
		var resp []string
		for _, r := range h.Rules {
			if r.Side == SideResponse {
				resp = append(resp, ruleText(r))
			}
		}
		if h.Security != nil {
			resp = append(resp, "security headers")
		}
		if h.CORS != nil {
			resp = append(resp, "CORS headers for allowed origins")
		}
		if h.HideServer {
			resp = append(resp, "remove Server and X-Powered-By")
		}
		if len(resp) > 0 {
			steps = append(steps, Step{Stage: KindHeaders, Outcome: "response headers: " + strings.Join(resp, "; ")})
		}
	}
	return steps
}

func forwardStep(fw *Forwarders, method, path string) Step {
	if fw != nil {
		for i, f := range fw.Rules {
			if !f.Match.Matches(method, path) {
				continue
			}
			label := fmt.Sprintf("forwarder %d (%s)", i+1, f.Match.Describe())
			switch f.Action {
			case ActionRedirect:
				status := f.RedirectStatus
				if status == 0 {
					status = 302
				}
				return Step{Stage: KindForwarders, Outcome: fmt.Sprintf("%s: %d redirect to %s", label, status, f.URL), Final: true}
			case ActionApp:
				return Step{Stage: KindForwarders, Outcome: fmt.Sprintf("%s: proxied to app %s as %s", label, f.App, ForwardedPath(f, path)), Final: true}
			default:
				return Step{Stage: KindForwarders, Outcome: fmt.Sprintf("%s: proxied to %s as %s", label, f.URL, ForwardedPath(f, path)), Final: true}
			}
		}
	}
	return Step{Stage: KindForwarders, Outcome: "proxied to this domain's app", Final: true}
}

// ForwardedPath is the path the upstream sees after strip and rewrite.
func ForwardedPath(f Forwarder, path string) string {
	out := path
	if f.StripPrefix && f.Match.Kind == MatchPrefix {
		out = strings.TrimPrefix(out, strings.TrimSuffix(f.Match.Path, "/"))
		if out == "" || out[0] != '/' {
			out = "/" + out
		}
	}
	prefix := f.RewritePrefix
	if prefix == "" && f.Action == ActionURL {
		if u, err := url.Parse(f.URL); err == nil {
			prefix = u.Path
		}
	}
	if prefix != "" && prefix != "/" {
		out = strings.TrimSuffix(prefix, "/") + out
	}
	return out
}

func ruleText(r HeaderRule) string {
	if r.Op == OpRemove {
		return "remove " + r.Name
	}
	return r.Op + " " + r.Name + ": " + r.Value
}

func cookieText(r CacheRule) string {
	if r.CacheWithCookies {
		return ""
	}
	return " or a Cookie"
}

func geoActionText(g Geo) string {
	switch g.Action {
	case GeoActionRedirect:
		return "redirected to " + g.RedirectURL
	case GeoActionErrorPage:
		return "custom page, status " + strconv.Itoa(g.BlockStatus())
	}
	return "status " + strconv.Itoa(g.BlockStatus())
}

func lastSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
