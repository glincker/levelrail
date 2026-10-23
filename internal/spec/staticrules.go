package spec

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// Static routing validation lives here, not in the package that serves it,
// so this foundational package does not import a feature package.

// StaticSourceSpec labels rules that come from app.yaml.
const StaticSourceSpec = "app.yaml"

// Trailing slash modes for StaticServeOptions.TrailingSlash.
const (
	StaticSlashOff    = "off"
	StaticSlashAdd    = "add"
	StaticSlashRemove = "remove"
)

const staticSplatName = "splat"

var (
	staticNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	// StaticPlaceholderRe matches a :name placeholder in a rule target.
	StaticPlaceholderRe = regexp.MustCompile(`:([A-Za-z_][A-Za-z0-9_]*)`)
	staticQueryKeyRe    = regexp.MustCompile(`^[A-Za-z0-9_.\-\[\]]+$`)
)

// staticAllowedStatuses are the codes a rule may use. 200 rewrites, 404/410/451
// serve the target page with that status, the rest redirect.
var staticAllowedStatuses = map[int]bool{200: true, 301: true, 302: true, 303: true, 307: true, 308: true, 404: true, 410: true, 451: true}

// IsStaticRedirectStatus reports whether s is a 3xx code.
func IsStaticRedirectStatus(s int) bool { return s >= 300 && s < 400 }

// StaticQueryMatch requires a query parameter; Value is a literal or ":name".
type StaticQueryMatch struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// StaticRule is one redirect, rewrite or status rule.
type StaticRule struct {
	Source string             `json:"source"`
	From   string             `json:"from"`
	To     string             `json:"to"`
	Status int                `json:"status"`
	Force  bool               `json:"force,omitempty"`
	Query  []StaticQueryMatch `json:"query,omitempty"`

	pat *StaticPattern
}

// Pattern returns the compiled From matcher, nil until validated.
func (r *StaticRule) Pattern() *StaticPattern { return r.pat }

// StaticHeaderKV is one response header.
type StaticHeaderKV struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// StaticHeaderBlock applies Set and Remove to responses whose path matches Path.
type StaticHeaderBlock struct {
	Source string           `json:"source"`
	Path   string           `json:"path"`
	Set    []StaticHeaderKV `json:"set,omitempty"`
	Remove []string         `json:"remove,omitempty"`

	pat *StaticPattern
}

// Pattern returns the compiled Path matcher, nil until validated.
func (b *StaticHeaderBlock) Pattern() *StaticPattern { return b.pat }

// SetPattern stores the matcher compiled for Path.
func (b *StaticHeaderBlock) SetPattern(p *StaticPattern) { b.pat = p }

// StaticServeOptions are the static serving switches.
type StaticServeOptions struct {
	SPA           bool   `json:"spa,omitempty"`
	CleanURLs     bool   `json:"clean_urls,omitempty"`
	TrailingSlash string `json:"trailing_slash,omitempty"`
	NotFound      string `json:"not_found,omitempty"`
}

// StaticLimits caps the size of a site's routing rules.
type StaticLimits struct {
	MaxRules        int
	MaxHeaderBlocks int
	MaxFileBytes    int
	MaxValueLen     int
}

// DefaultStaticLimits are the caps used when no environment override is set.
func DefaultStaticLimits() StaticLimits {
	return StaticLimits{MaxRules: 1000, MaxHeaderBlocks: 100, MaxFileBytes: 256 << 10, MaxValueLen: 2048}
}

// StaticPattern is a path pattern with :name segments and a trailing * splat,
// compiled to an RE2 expression so matching is linear in the path length.
type StaticPattern struct {
	re    *regexp.Regexp
	names []string
}

// CompileStaticPattern compiles a path pattern.
func CompileStaticPattern(p string) (*StaticPattern, error) {
	if !strings.HasPrefix(p, "/") {
		return nil, fmt.Errorf("path %q must start with /", p)
	}
	trimmed := strings.TrimSuffix(p, "/")
	var segs []string
	if trimmed != "" {
		segs = strings.Split(trimmed[1:], "/")
	}
	pt := &StaticPattern{}
	seen := map[string]bool{}
	var b strings.Builder
	b.WriteString("(?is)^")
	for i, seg := range segs {
		switch {
		case seg == "":
			return nil, fmt.Errorf("path %q has an empty segment", p)
		case seg == "*":
			if i != len(segs)-1 {
				return nil, fmt.Errorf("path %q: * is only allowed as the last segment", p)
			}
			b.WriteString("(?:/(.*))?")
			pt.names = append(pt.names, staticSplatName)
		case strings.HasPrefix(seg, ":"):
			name := seg[1:]
			if !staticNameRe.MatchString(name) {
				return nil, fmt.Errorf("path %q: placeholder %q must be :name with letters, digits and _", p, seg)
			}
			if name == staticSplatName || seen[name] {
				return nil, fmt.Errorf("path %q: placeholder :%s is reserved or repeated", p, name)
			}
			seen[name] = true
			b.WriteString("/([^/]+)")
			pt.names = append(pt.names, name)
		case strings.ContainsAny(seg, "*:"):
			return nil, fmt.Errorf("path %q: * and : must be a whole segment", p)
		default:
			b.WriteString("/" + regexp.QuoteMeta(seg))
		}
	}
	if len(segs) == 0 {
		b.WriteString("/")
	} else {
		b.WriteString("/?")
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, fmt.Errorf("path %q: %w", p, err)
	}
	pt.re = re
	return pt, nil
}

// Match returns the captured values, or nil when path does not match.
func (pt *StaticPattern) Match(path string) map[string]string {
	m := pt.re.FindStringSubmatch(path)
	if m == nil {
		return nil
	}
	caps := make(map[string]string, len(pt.names))
	for i, n := range pt.names {
		caps[n] = m[i+1]
	}
	return caps
}

// ValidateStaticRule checks a rule and prepares its matcher. It is the single
// gate for file rules and app.yaml rules alike.
func ValidateStaticRule(r *StaticRule, l StaticLimits) error {
	if r.Status == 0 {
		r.Status = 301
	}
	if !staticAllowedStatuses[r.Status] {
		return fmt.Errorf("status %d is not supported (use 200, 301, 302, 303, 307, 308, 404, 410 or 451)", r.Status)
	}
	if r.From == "" || r.To == "" {
		return fmt.Errorf("from and to are required")
	}
	if len(r.From) > l.MaxValueLen || len(r.To) > l.MaxValueLen {
		return fmt.Errorf("from and to must be at most %d bytes", l.MaxValueLen)
	}
	for _, v := range []string{r.From, r.To} {
		if !SafeHeaderValue(v) || strings.ContainsAny(v, " \t\\") {
			return fmt.Errorf("%q must not contain spaces, backslashes or control characters", v)
		}
	}
	pat, err := CompileStaticPattern(r.From)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, n := range pat.names {
		known[n] = true
	}
	for _, q := range r.Query {
		if !staticQueryKeyRe.MatchString(q.Key) || q.Value == "" || len(q.Value) > l.MaxValueLen {
			return fmt.Errorf("query match %q must be key=value or key=:name", q.Key+"="+q.Value)
		}
		if strings.HasPrefix(q.Value, ":") {
			if !staticNameRe.MatchString(q.Value[1:]) || known[q.Value[1:]] {
				return fmt.Errorf("query placeholder %q is invalid or repeats a path placeholder", q.Value)
			}
			known[q.Value[1:]] = true
		}
	}
	if err := validateStaticTarget(r, known); err != nil {
		return err
	}
	r.pat = pat
	return nil
}

func validateStaticTarget(r *StaticRule, known map[string]bool) error {
	pathLike := strings.HasPrefix(r.To, "/")
	if pathLike {
		if strings.HasPrefix(r.To, "//") {
			return fmt.Errorf("to %q must not start with // (that would be a redirect to another host)", r.To)
		}
	} else {
		if r.Status == 200 {
			return fmt.Errorf("a 200 rewrite to an external URL is a proxy and is not supported; use a path forwarder on the domain")
		}
		if !IsStaticRedirectStatus(r.Status) {
			return fmt.Errorf("status %d needs a path on this site as its destination", r.Status)
		}
		if err := validateStaticAbsoluteURL(r.To); err != nil {
			return err
		}
	}
	if IsStaticRedirectStatus(r.Status) && pathLike && len(r.Query) == 0 && !strings.ContainsAny(r.From, ":*") &&
		!strings.ContainsAny(r.To, ":?") && strings.EqualFold(strings.TrimSuffix(r.To, "/"), strings.TrimSuffix(r.From, "/")) {
		return fmt.Errorf("redirects %q to itself, which would loop", r.From)
	}
	for _, m := range StaticPlaceholderRe.FindAllStringSubmatch(staticAfterScheme(r.To), -1) {
		if !known[m[1]] {
			return fmt.Errorf("to uses :%s but from has no such placeholder", m[1])
		}
	}
	return nil
}

func staticAfterScheme(to string) string {
	if i := strings.Index(to, "://"); i >= 0 {
		return to[i+3:]
	}
	return to
}

func validateStaticAbsoluteURL(raw string) error {
	hostPart := strings.SplitN(staticAfterScheme(raw), "/", 2)[0]
	hostPart = strings.SplitN(strings.SplitN(hostPart, "?", 2)[0], "#", 2)[0]
	hostPart = hostPart[strings.LastIndex(hostPart, "@")+1:]
	if strings.Contains(raw, "://") && (StaticPlaceholderRe.MatchString(hostPart) || strings.Contains(hostPart, "*")) {
		return fmt.Errorf("the host of an absolute destination must be fixed text, not a placeholder or splat")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return fmt.Errorf("to %q must be a path starting with / or an absolute http(s) URL", raw)
	}
	if u.User != nil {
		return fmt.Errorf("to must not contain credentials")
	}
	return nil
}

// ValidateStaticHeaderPath checks a header block path and compiles it.
func ValidateStaticHeaderPath(p string, l StaticLimits) (*StaticPattern, error) {
	if len(p) > l.MaxValueLen || !SafeHeaderValue(p) || strings.ContainsAny(p, " \t") {
		return nil, fmt.Errorf("path %q must be a single path without spaces", p)
	}
	return CompileStaticPattern(p)
}

// AddStaticHeaderLine parses one "Name: value" or "! Name" line into b.
func AddStaticHeaderLine(b *StaticHeaderBlock, line string, l StaticLimits) error {
	if strings.HasPrefix(line, "!") {
		name := strings.TrimSpace(line[1:])
		if err := checkStaticHeaderName(name); err != nil {
			return err
		}
		b.Remove = append(b.Remove, http.CanonicalHeaderKey(name))
		return nil
	}
	name, value, ok := strings.Cut(line, ":")
	name, value = strings.TrimSpace(name), strings.TrimSpace(value)
	if !ok || value == "" {
		return fmt.Errorf("%q must be Name: value", line)
	}
	if err := checkStaticHeaderName(name); err != nil {
		return err
	}
	if len(value) > l.MaxValueLen || !SafeHeaderValue(value) {
		return fmt.Errorf("value of %s must be a single line of at most %d bytes", name, l.MaxValueLen)
	}
	b.Set = append(b.Set, StaticHeaderKV{Name: http.CanonicalHeaderKey(name), Value: value})
	return nil
}

func checkStaticHeaderName(name string) error {
	if name == "" {
		return fmt.Errorf("header name is empty")
	}
	if msg := HeaderNameProblem(HeaderSideResponse, name); msg != "" {
		return fmt.Errorf("header %q: %s", name, msg)
	}
	return nil
}

// ValidateStaticHeaderBlock checks a block built from app.yaml and prepares it.
func ValidateStaticHeaderBlock(b *StaticHeaderBlock, l StaticLimits) error {
	pat, err := ValidateStaticHeaderPath(b.Path, l)
	if err != nil {
		return err
	}
	scratch := StaticHeaderBlock{}
	for _, kv := range b.Set {
		if err := AddStaticHeaderLine(&scratch, kv.Name+": "+kv.Value, l); err != nil {
			return err
		}
	}
	for _, name := range b.Remove {
		if err := AddStaticHeaderLine(&scratch, "! "+name, l); err != nil {
			return err
		}
	}
	b.Set, b.Remove, b.pat = scratch.Set, scratch.Remove, pat
	return nil
}

// ValidateStaticOptions checks the serving switches.
func ValidateStaticOptions(o *StaticServeOptions) error {
	switch o.TrailingSlash {
	case "", StaticSlashOff, StaticSlashAdd, StaticSlashRemove:
	default:
		return fmt.Errorf("static.trailing_slash must be off, add or remove")
	}
	if o.NotFound != "" && (!filepath.IsLocal(o.NotFound) || len(o.NotFound) > 255) {
		return fmt.Errorf("static.not_found must be a file path inside the published directory")
	}
	return nil
}

// CompileStaticEntries validates the app.yaml routes and header blocks and
// returns them with matchers prepared and Source set.
func CompileStaticEntries(routes []StaticRule, blocks []StaticHeaderBlock, opts *StaticServeOptions, l StaticLimits) ([]StaticRule, []StaticHeaderBlock, error) {
	if err := ValidateStaticOptions(opts); err != nil {
		return nil, nil, err
	}
	rules := make([]StaticRule, 0, len(routes))
	for i := range routes {
		r := routes[i]
		r.Source = fmt.Sprintf("%s routes[%d]", StaticSourceSpec, i)
		if err := ValidateStaticRule(&r, l); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", r.Source, err)
		}
		rules = append(rules, r)
	}
	headers := make([]StaticHeaderBlock, 0, len(blocks))
	for i := range blocks {
		b := blocks[i]
		b.Source = fmt.Sprintf("%s headers[%d]", StaticSourceSpec, i)
		if err := ValidateStaticHeaderBlock(&b, l); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", b.Source, err)
		}
		headers = append(headers, b)
	}
	return rules, headers, nil
}

// Environment variables that tune the rule-count caps in StaticLimits.
const (
	EnvStaticMaxRules        = "APP_STATIC_ROUTES_MAX_RULES"
	EnvStaticMaxHeaderBlocks = "APP_STATIC_ROUTES_MAX_HEADER_BLOCKS"
	EnvStaticMaxFileBytes    = "APP_STATIC_ROUTES_MAX_FILE_BYTES"
)
