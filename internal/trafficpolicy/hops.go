package trafficpolicy

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// HostRedirect is one domain_redirect row: every request to the host is
// sent to TargetURL, keeping path and query when TargetURL is a bare origin.
type HostRedirect struct {
	TargetURL  string `json:"target_url"`
	StatusCode int    `json:"status_code"`
}

// HopEnv is everything the redirect simulation needs about the instance.
type HopEnv struct {
	// Hosts routed by this instance, lower case.
	Hosts map[string]bool
	// Redirects maps a host to its normalisation settings.
	Redirects map[string]Redirects
	// HostRedirects maps a host to its whole-host redirect.
	HostRedirects map[string]HostRedirect
	// Maintenance hosts answer 503 before any redirect.
	Maintenance map[string]bool
	// AutoHTTPS is true when the ingress itself redirects every http
	// request to https (the HTTP listener is bound and TLS ends here).
	AutoHTTPS bool
	// Terminated is true when a proxy in front terminates TLS; force HTTPS
	// then trusts its X-Forwarded-Proto.
	Terminated bool
	// HTTPSPort is the public HTTPS port; 0 or 443 is omitted from URLs.
	HTTPSPort int
}

// Hop is one response in a redirect chain.
type Hop struct {
	URL      string `json:"url"`
	Status   int    `json:"status"`
	Location string `json:"location,omitempty"`
	Note     string `json:"note"`
}

// MaxHops bounds a simulated chain; longer chains are reported as loops.
const MaxHops = 8

// SimulateHops follows rawURL through the ingress's redirect behavior.
func SimulateHops(env HopEnv, rawURL string) ([]Hop, error) {
	seen := map[string]bool{}
	current := rawURL
	var hops []Hop
	for i := 0; i < MaxHops; i++ {
		if seen[current] {
			return hops, fmt.Errorf("redirect loop: %s is reached twice", current)
		}
		seen[current] = true
		hop, next := env.step(current)
		hops = append(hops, hop)
		if next == "" {
			return hops, nil
		}
		current = next
	}
	return hops, fmt.Errorf("more than %d redirects", MaxHops)
}

func (env HopEnv) step(raw string) (Hop, string) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return Hop{URL: raw, Note: "not a valid URL"}, ""
	}
	host := u.Hostname()
	lower := strings.ToLower(host)
	if !env.Hosts[lower] {
		return Hop{URL: raw, Note: "not served by this instance"}, ""
	}
	settings := env.Redirects[lower]
	if host != lower && settings.LowercaseHost {
		next := *u
		next.Host = strings.Replace(u.Host, host, lower, 1)
		return redirectHop(raw, 308, next.String(), "host lower cased"), next.String()
	}
	if env.Maintenance[lower] {
		return Hop{URL: raw, Status: 503, Note: "maintenance mode"}, ""
	}
	if u.Scheme == "http" {
		switch {
		case env.Terminated && settings.ForceHTTPS:
			next := env.httpsURL(u, lower)
			return redirectHop(raw, settings.HTTPSStatus(), next, "force HTTPS (your proxy marks the request as http)"), next
		case env.AutoHTTPS:
			next := env.httpsURL(u, lower)
			return redirectHop(raw, 308, next, "automatic HTTPS"), next
		case !env.Terminated:
			return Hop{URL: raw, Note: "plain HTTP is not served by this instance"}, ""
		}
	}
	if r, ok := env.HostRedirects[lower]; ok {
		next := RedirectTarget(r.TargetURL, u)
		status := r.StatusCode
		if status == 0 {
			status = 301
		}
		return redirectHop(raw, status, next, "domain redirect"), next
	}
	switch settings.SlashMode() {
	case SlashAdd:
		if !strings.HasSuffix(u.Path, "/") && !strings.Contains(lastSegment(u.Path), ".") {
			next := *u
			next.Path = u.Path + "/"
			return redirectHop(raw, 308, next.String(), "trailing slash added"), next.String()
		}
	case SlashRemove:
		if u.Path != "/" && u.Path != "" && strings.HasSuffix(u.Path, "/") {
			next := *u
			next.Path = strings.TrimSuffix(u.Path, "/")
			return redirectHop(raw, 308, next.String(), "trailing slash removed"), next.String()
		}
	}
	return Hop{URL: raw, Status: 200, Note: "served by the app"}, ""
}

func redirectHop(raw string, status int, location, note string) Hop {
	return Hop{URL: raw, Status: status, Location: location, Note: note}
}

func (env HopEnv) httpsURL(u *url.URL, host string) string {
	next := *u
	next.Scheme = "https"
	next.Host = host
	if env.HTTPSPort != 0 && env.HTTPSPort != 443 {
		next.Host = host + ":" + strconv.Itoa(env.HTTPSPort)
	}
	return next.String()
}

// RedirectTarget mirrors the ingress: a bare origin target keeps the
// request's path and query, a target with its own path is used as is.
func RedirectTarget(target string, req *url.URL) string {
	t, err := url.Parse(target)
	if err != nil || (t.Path != "" && t.Path != "/") || t.RawQuery != "" || t.Fragment != "" {
		return target
	}
	return strings.TrimSuffix(target, "/") + req.RequestURI()
}
