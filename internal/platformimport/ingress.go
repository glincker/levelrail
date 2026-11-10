package platformimport

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"regexp/syntax"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// Environment variables that cap how much untrusted proxy label input the importer reads.
const (
	MaxLabelBytesEnv     = "APP_IMPORT_MAX_LABEL_BYTES"
	MaxRegexInstsEnv     = "APP_IMPORT_MAX_REGEX_INSTS"
	MaxRedirectURLLenEnv = "APP_IMPORT_MAX_REDIRECT_URL_LEN"
	OpaqueValueMinEnv    = "APP_IMPORT_OPAQUE_VALUE_MIN_LEN"
)

const (
	defaultMaxLabelBytes     = 64 << 10
	defaultMaxRegexInsts     = 4096
	defaultMaxRedirectURLLen = 2048
	defaultOpaqueValueMin    = 20
	maxLabelKeyShown         = 128
)

// IngressPlan is the ingress settings translated from a source app's proxy settings. It never holds
// a credential: basic auth is a flag, and a header with a secret-looking name or value is left manual.
type IngressPlan struct {
	// BasicAuth gates every domain with a placeholder credential the operator replaces.
	BasicAuth bool `json:"basic_auth,omitempty"`
	// Canonical is a www redirect preset (www-to-apex or apex-to-www).
	Canonical  string                     `json:"canonical,omitempty"`
	ForceHTTPS bool                       `json:"force_https,omitempty"`
	Forwarders []trafficpolicy.Forwarder  `json:"forwarders,omitempty"`
	Headers    []trafficpolicy.HeaderRule `json:"headers,omitempty"`
	// Manual lists label keys and settings with no translation, each with a reason; values are never kept.
	Manual []Note `json:"manual,omitempty"`
}

// Translated reports whether the plan carries any setting to apply.
func (p IngressPlan) Translated() bool {
	return p.BasicAuth || p.Canonical != "" || p.ForceHTTPS || len(p.Forwarders) > 0 || len(p.Headers) > 0
}

// IngressSource is the proxy configuration read from one source app.
type IngressSource struct {
	Labels    string
	BasicAuth bool
	// Redirect is the source's www rule: www, non-www or both.
	Redirect string
	// App is the target app name a strip-prefix rule forwards back to.
	App string
	// Domain is the first domain, used as the validation context.
	Domain string
}

type redirectMW struct{ regex, replacement, permanent string }

// TranslateIngress turns basic auth, the www rule and common proxy label middleware into ingress
// settings. Every value is untrusted, so each rule passes the domain policy validators or stays manual.
func TranslateIngress(src IngressSource, getenv func(string) string) IngressPlan {
	var p IngressPlan
	if getenv == nil {
		getenv = os.Getenv
	}
	limits := trafficpolicy.LimitsFromEnv(getenv)
	ctx := trafficpolicy.Context{Domain: strings.ToLower(src.Domain)}
	manual := func(reason, todo string) {
		p.Manual = append(p.Manual, Note{Area: "ingress", Reason: reason, Manual: todo})
	}
	p.BasicAuth = src.BasicAuth
	switch r := strings.ToLower(strings.TrimSpace(src.Redirect)); r {
	case "", "both":
	case "www":
		p.Canonical = trafficpolicy.PresetApexToWWW
	case "non-www":
		p.Canonical = trafficpolicy.PresetWWWToApex
	default:
		manual("www redirect rule "+shownKey(r)+" has no translation", "add the redirect in the app's domain settings")
	}
	redirects := map[string]*redirectMW{}
	for _, kv := range parseLabels(src.Labels, envInt(getenv, MaxLabelBytesEnv, defaultMaxLabelBytes), manual) {
		translateLabel(&p, kv[0], kv[1], src.App, redirects, manual)
	}
	names := make([]string, 0, len(redirects))
	for n := range redirects {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if f, why := redirectForwarder(n, *redirects[n], ctx.Domain, getenv, limits); why != "" {
			manual("redirectregex middleware "+shownKey(n)+" was not translated: "+why, "add the redirect as a forwarder rule in the domain settings")
		} else {
			p.Forwarders = append(p.Forwarders, f)
		}
	}
	p.Forwarders = keepValid(p.Forwarders, func(f trafficpolicy.Forwarder) error {
		return (&trafficpolicy.Forwarders{Rules: []trafficpolicy.Forwarder{f}}).Validate(limits, ctx)
	}, func(f trafficpolicy.Forwarder, err error) {
		manual("rule "+shownKey(f.Name)+" failed validation: "+err.Error(), "recreate it in the domain settings")
	})
	p.Headers = keepValid(p.Headers, func(h trafficpolicy.HeaderRule) error {
		return (&trafficpolicy.Headers{Rules: []trafficpolicy.HeaderRule{h}}).Validate(limits, ctx)
	}, func(h trafficpolicy.HeaderRule, _ error) {
		manual("header "+shownKey(h.Name)+" failed validation", "set it in the domain's header settings")
	})
	if maxRules := limits.MaxHeaderRules; len(p.Headers) > maxRules {
		manual(fmt.Sprintf("%d header rules exceed the per-domain cap of %d, the rest were dropped", len(p.Headers), maxRules), "set the remaining headers by hand")
		p.Headers = p.Headers[:maxRules]
	}
	return p
}

// ingressNotes reports what the plan sets and what stays manual, never a value from the source.
func ingressNotes(p IngressPlan) []Note {
	var notes []Note
	planned := func(reason, manual string) {
		notes = append(notes, Note{Area: "ingress", Reason: "planned: " + reason, Manual: manual})
	}
	if p.BasicAuth {
		planned("basic auth was on, every domain is gated with a placeholder credential that is never shown", "set the real credential with domains basic-auth set before routing traffic")
	}
	if p.Canonical != "" {
		planned("the www rule becomes the "+p.Canonical+" canonical redirect", "check it in the domain's redirect settings")
	}
	if p.ForceHTTPS {
		planned("the redirect to HTTPS becomes the force HTTPS setting", "none")
	}
	if n := len(p.Forwarders); n > 0 {
		planned(fmt.Sprintf("%d redirect or strip-prefix label rule(s) become forwarder rules", n), "check them in the domain's forwarder settings")
	}
	if n := len(p.Headers); n > 0 {
		planned(fmt.Sprintf("%d header label rule(s) become header rules", n), "check them in the domain's header settings")
	}
	return append(notes, p.Manual...)
}

func keepValid[T any](in []T, check func(T) error, drop func(T, error)) []T {
	out := in[:0]
	for _, v := range in {
		if err := check(v); err != nil {
			drop(v, err)
			continue
		}
		out = append(out, v)
	}
	return out
}

// parseLabels reads KEY=VALUE lines, plain or base64 encoded as the source stores them.
func parseLabels(raw string, maxBytes int, manual func(string, string)) [][2]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if len(raw) > maxBytes {
		manual(fmt.Sprintf("custom labels are larger than %d bytes and were not read", maxBytes), "recreate redirects, headers or auth as ingress settings by hand")
		return nil
	}
	if dec, err := base64.StdEncoding.DecodeString(raw); err == nil && utf8.Valid(dec) && strings.Contains(string(dec), "=") {
		raw = string(dec)
	}
	var out [][2]string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, _ := strings.Cut(line, "=")
		out = append(out, [2]string{strings.Trim(strings.TrimSpace(k), `"'`), strings.Trim(strings.TrimSpace(v), `"'`)})
	}
	return out
}

func translateLabel(p *IngressPlan, key, value, app string, redirects map[string]*redirectMW, manual func(string, string)) {
	lk := strings.ToLower(key)
	switch {
	case lk == "traefik.enable", strings.HasPrefix(lk, "traefik.http.routers."), strings.HasPrefix(lk, "traefik.http.services."),
		strings.HasPrefix(lk, "traefik.docker."), strings.HasPrefix(lk, "coolify."):
		return // routing and service wiring come from the app's domains and port
	case !strings.HasPrefix(lk, "traefik.http.middlewares."):
		manual("label "+shownKey(key)+" has no translation here", "recreate its effect as an ingress or app setting by hand")
		return
	}
	parts := strings.Split(key, ".")
	if len(parts) < 6 {
		manual("label "+shownKey(key)+" is not a complete middleware option", "recreate its effect by hand")
		return
	}
	name, typ, opt := parts[3], strings.ToLower(parts[4]), strings.Join(parts[5:], ".")
	switch typ {
	case "redirectscheme":
		if strings.EqualFold(opt, "scheme") && strings.EqualFold(value, "https") {
			p.ForceHTTPS = true
		}
	case "redirectregex":
		r := redirects[name]
		if r == nil {
			if len(redirects) >= 64 {
				manual("too many redirectregex middlewares, "+shownKey(name)+" was skipped", "add it as a forwarder rule by hand")
				return
			}
			r = &redirectMW{}
			redirects[name] = r
		}
		switch strings.ToLower(opt) {
		case "regex":
			r.regex = value
		case "replacement":
			r.replacement = value
		case "permanent":
			r.permanent = value
		}
	case "stripprefix":
		if !strings.EqualFold(opt, "prefixes") {
			return
		}
		for _, pre := range strings.Split(value, ",") {
			pre = strings.TrimSpace(pre)
			p.Forwarders = append(p.Forwarders, trafficpolicy.Forwarder{Name: "imported-" + SanitizeName(name),
				Match: trafficpolicy.Match{Kind: trafficpolicy.MatchPrefix, Path: pre}, Action: trafficpolicy.ActionApp, App: app, StripPrefix: true})
		}
	case "headers":
		translateHeader(p, key, opt, value, manual)
	case "basicauth":
		// The user list is a credential: it is never read, the domain gets a placeholder instead.
		p.BasicAuth = true
	default:
		manual("middleware "+shownKey(name)+" of type "+shownKey(typ)+" has no translation here", "recreate it as an ingress setting by hand")
	}
}

var sensitiveHeaders = map[string]bool{"authorization": true, "proxy-authorization": true, "cookie": true, "set-cookie": true, "x-api-key": true, "x-auth-token": true}

func translateHeader(p *IngressPlan, key, opt, value string, manual func(string, string)) {
	group, header, ok := strings.Cut(opt, ".")
	side := ""
	switch strings.ToLower(group) {
	case "customrequestheaders":
		side = trafficpolicy.SideRequest
	case "customresponseheaders":
		side = trafficpolicy.SideResponse
	}
	if !ok || side == "" {
		manual("header option "+shownKey(key)+" has no translation here", "use the security headers preset or a header rule")
		return
	}
	if sensitiveHeaders[strings.ToLower(header)] || secretName(header) || CommandHasSecret(header+": "+value) || LooksSecret(header) ||
		ValueLooksSecret(value) || tokenLike(value) {
		manual("header "+shownKey(header)+" carries a secret-looking value, it was not copied", "set it in the domain's header settings by hand")
		return
	}
	rule := trafficpolicy.HeaderRule{Side: side, Op: trafficpolicy.OpSet, Name: header, Value: value}
	if value == "" {
		rule.Op, rule.Value = trafficpolicy.OpRemove, ""
	}
	p.Headers = append(p.Headers, rule)
}

var hostPrefixRe = regexp.MustCompile(`^\^[A-Za-z()?|\\]*://([^/]*)`)

// redirectForwarder builds a redirect rule, or returns why the middleware stays manual.
func redirectForwarder(name string, r redirectMW, domain string, getenv func(string) string, l trafficpolicy.Limits) (trafficpolicy.Forwarder, string) {
	var f trafficpolicy.Forwarder
	if r.regex == "" || r.replacement == "" {
		return f, "it needs both a regex and a replacement"
	}
	if len(r.regex) > l.MaxRegexLen {
		return f, fmt.Sprintf("the regex is longer than %d bytes", l.MaxRegexLen)
	}
	path := r.regex
	if !strings.HasPrefix(path, "^/") {
		m := hostPrefixRe.FindStringSubmatch(path)
		if m == nil {
			return f, "the regex does not match from the start of the URL or path"
		}
		// The rule lands on the app's first domain, so a regex scoped to any other host must not widen to it.
		host := strings.ToLower(strings.ReplaceAll(m[1], `\.`, "."))
		if strings.ContainsAny(host, `()[]?*+|^$\{}`) || host == "" || host != domain {
			return f, "the regex is scoped to another or a pattern host, not the app's first domain"
		}
		path = "^" + path[len(m[0]):]
	}
	if !strings.HasPrefix(path, "^/") {
		return f, "the regex does not match from the start of the URL or path"
	}
	re, err := syntax.Parse(path, syntax.Perl)
	if err != nil {
		return f, "the regex does not compile"
	}
	prog, err := syntax.Compile(re.Simplify())
	if err != nil || len(prog.Inst) > envInt(getenv, MaxRegexInstsEnv, defaultMaxRegexInsts) {
		return f, "the regex is too complex"
	}
	if strings.Contains(r.replacement, "$") {
		return f, "the replacement uses capture groups, which a redirect rule here does not expand"
	}
	if len(r.replacement) > envInt(getenv, MaxRedirectURLLenEnv, defaultMaxRedirectURLLen) || !trafficpolicy.SafeHeaderValue(r.replacement) {
		return f, "the replacement is too long or holds control characters"
	}
	u, err := url.Parse(r.replacement)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return f, "the replacement is not an absolute http or https URL"
	}
	if u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return f, "the replacement has a query or fragment, which may carry a token"
	}
	status := 302
	if b, err := strconv.ParseBool(r.permanent); err == nil && b {
		status = 308
	}
	return trafficpolicy.Forwarder{Name: "imported-" + SanitizeName(name), Match: trafficpolicy.Match{Kind: trafficpolicy.MatchRegex, Path: path},
		Action: trafficpolicy.ActionRedirect, URL: u.String(), RedirectStatus: status}, ""
}

// tokenLike flags a long value with no spaces that mixes letters and digits, the shape of a random token.
func tokenLike(v string) bool {
	if len(v) < envInt(nil, OpaqueValueMinEnv, defaultOpaqueValueMin) || strings.ContainsAny(v, " ,;=") {
		return false
	}
	var letters, digits bool
	for _, r := range v {
		letters = letters || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		digits = digits || (r >= '0' && r <= '9')
	}
	return letters && digits
}

// shownKey makes an untrusted label key safe to print: printable runes only, length capped.
func shownKey(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	if len(s) > maxLabelKeyShown {
		s = s[:maxLabelKeyShown] + "..."
	}
	return strconv.Quote(s)
}

func envInt(getenv func(string) string, key string, def int) int {
	if getenv == nil {
		getenv = os.Getenv
	}
	if v, err := strconv.Atoi(strings.TrimSpace(getenv(key))); err == nil && v > 0 {
		return v
	}
	return def
}
