package staticroutes

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

var (
	statusTokenRe = regexp.MustCompile(`^([0-9]{3})(!?)$`)
	queryKeyRe    = regexp.MustCompile(`^[A-Za-z0-9_.\-\[\]]+$`)
)

// allowedStatuses are the codes a rule may use. 200 rewrites, 404/410/451
// serve the target page with that status, the rest redirect.
var allowedStatuses = map[int]bool{200: true, 301: true, 302: true, 303: true, 307: true, 308: true, 404: true, 410: true, 451: true}

func isRedirectStatus(s int) bool { return s >= 300 && s < 400 }

func isQueryToken(tok string) bool {
	return strings.Contains(tok, "=") && !strings.HasPrefix(tok, "/") && !strings.HasPrefix(tok, "http://") && !strings.HasPrefix(tok, "https://")
}

// ParseRedirects parses Netlify _redirects syntax: "from [key=val ...] to
// [status[!]]" per line. Bad lines are skipped and reported, never applied
// half understood.
func ParseRedirects(src string, l Limits) ([]Rule, []Problem) {
	var rules []Rule
	var probs []Problem
	for i, line := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lineNo := i + 1
		bad := func(format string, a ...any) {
			probs = append(probs, Problem{Source: SourceRedirects, Line: lineNo, Severity: SeverityError, Message: fmt.Sprintf(format, a...)})
		}
		r, err := parseRedirectLine(line)
		if err != nil {
			bad("%s", err)
			continue
		}
		r.Source = fmt.Sprintf("%s:%d", SourceRedirects, lineNo)
		if err := ValidateRule(&r, l); err != nil {
			bad("%s", err)
			continue
		}
		rules = append(rules, r)
	}
	return rules, probs
}

func parseRedirectLine(line string) (Rule, error) {
	f := strings.Fields(line)
	r := Rule{From: f[0], Status: 301}
	if !strings.HasPrefix(r.From, "/") {
		return r, fmt.Errorf("from %q must be a path starting with /; host based rules are not supported", r.From)
	}
	rest := f[1:]
	for len(rest) > 0 && isQueryToken(rest[0]) {
		k, v, _ := strings.Cut(rest[0], "=")
		r.Query = append(r.Query, QueryMatch{Key: k, Value: v})
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return r, fmt.Errorf("missing the destination after %q", r.From)
	}
	r.To, rest = rest[0], rest[1:]
	for i, tok := range rest {
		if m := statusTokenRe.FindStringSubmatch(tok); m != nil && i == 0 {
			r.Status, _ = strconv.Atoi(m[1])
			r.Force = m[2] == "!"
			continue
		}
		if strings.Contains(tok, "=") {
			return r, fmt.Errorf("condition %q is not supported (country, language, role and cookie conditions need an edge runtime)", tok)
		}
		return r, fmt.Errorf("unexpected %q, expected a status code such as 301, with a ! suffix to force it", tok)
	}
	return r, nil
}

// ValidateRule checks a rule and prepares its matcher. It is the single
// gate for file rules and app.yaml rules alike.
func ValidateRule(r *Rule, l Limits) error {
	if r.Status == 0 {
		r.Status = 301
	}
	if !allowedStatuses[r.Status] {
		return fmt.Errorf("status %d is not supported (use 200, 301, 302, 303, 307, 308, 404, 410 or 451)", r.Status)
	}
	if r.From == "" || r.To == "" {
		return fmt.Errorf("from and to are required")
	}
	if len(r.From) > l.MaxValueLen || len(r.To) > l.MaxValueLen {
		return fmt.Errorf("from and to must be at most %d bytes", l.MaxValueLen)
	}
	for _, v := range []string{r.From, r.To} {
		if !trafficpolicy.SafeHeaderValue(v) || strings.ContainsAny(v, " \t\\") {
			return fmt.Errorf("%q must not contain spaces, backslashes or control characters", v)
		}
	}
	pat, err := compilePattern(r.From)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, n := range pat.names {
		known[n] = true
	}
	for _, q := range r.Query {
		if !queryKeyRe.MatchString(q.Key) || q.Value == "" || len(q.Value) > l.MaxValueLen {
			return fmt.Errorf("query match %q must be key=value or key=:name", q.Key+"="+q.Value)
		}
		if strings.HasPrefix(q.Value, ":") {
			if !nameRe.MatchString(q.Value[1:]) || known[q.Value[1:]] {
				return fmt.Errorf("query placeholder %q is invalid or repeats a path placeholder", q.Value)
			}
			known[q.Value[1:]] = true
		}
	}
	if err := validateTarget(r, known); err != nil {
		return err
	}
	r.pat = pat
	return nil
}

func validateTarget(r *Rule, known map[string]bool) error {
	pathLike := strings.HasPrefix(r.To, "/")
	if pathLike {
		if strings.HasPrefix(r.To, "//") {
			return fmt.Errorf("to %q must not start with // (that would be a redirect to another host)", r.To)
		}
	} else {
		if r.Status == 200 {
			return fmt.Errorf("a 200 rewrite to an external URL is a proxy and is not supported; use a path forwarder on the domain")
		}
		if !isRedirectStatus(r.Status) {
			return fmt.Errorf("status %d needs a path on this site as its destination", r.Status)
		}
		if err := validateAbsoluteURL(r.To); err != nil {
			return err
		}
	}
	if isRedirectStatus(r.Status) && pathLike && len(r.Query) == 0 && !strings.ContainsAny(r.From, ":*") &&
		!strings.ContainsAny(r.To, ":?") && strings.EqualFold(strings.TrimSuffix(r.To, "/"), strings.TrimSuffix(r.From, "/")) {
		return fmt.Errorf("redirects %q to itself, which would loop", r.From)
	}
	for _, m := range placeholderRe.FindAllStringSubmatch(afterScheme(r.To), -1) {
		if !known[m[1]] {
			return fmt.Errorf("to uses :%s but from has no such placeholder", m[1])
		}
	}
	return nil
}

func afterScheme(to string) string {
	if i := strings.Index(to, "://"); i >= 0 {
		return to[i+3:]
	}
	return to
}

func validateAbsoluteURL(raw string) error {
	hostPart := strings.SplitN(afterScheme(raw), "/", 2)[0]
	hostPart = strings.SplitN(strings.SplitN(hostPart, "?", 2)[0], "#", 2)[0]
	hostPart = hostPart[strings.LastIndex(hostPart, "@")+1:]
	if strings.Contains(raw, "://") && (placeholderRe.MatchString(hostPart) || strings.Contains(hostPart, "*")) {
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
