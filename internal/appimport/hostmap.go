package appimport

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

var hostTokenRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`)

// ValidateMapping checks both sides are plain hostnames.
func ValidateMapping(m Mapping) error {
	if !hostTokenRe.MatchString(m.From) {
		return fmt.Errorf("mapping source %q must be a hostname", m.From)
	}
	if !hostTokenRe.MatchString(m.To) {
		return fmt.Errorf("mapping target %q must be a hostname", m.To)
	}
	if strings.EqualFold(m.From, m.To) {
		return errors.New("mapping source and target are the same")
	}
	return nil
}

func isHostChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_'
}

// hostSpans returns the [start,end) byte ranges where host appears in v as
// a whole hostname token, so "db" does not match "db-replica" or "mydb".
func hostSpans(v, host string) [][2]int {
	if host == "" || v == "" {
		return nil
	}
	lv, lh := strings.ToLower(v), strings.ToLower(host)
	var out [][2]int
	for from := 0; from <= len(lv)-len(lh); {
		i := strings.Index(lv[from:], lh)
		if i < 0 {
			break
		}
		start := from + i
		end := start + len(lh)
		from = end
		if start > 0 && (isHostChar(lv[start-1]) || lv[start-1] == '.') {
			continue
		}
		if end < len(lv) && (isHostChar(lv[end]) || (lv[end] == '.' && end+1 < len(lv) && isHostChar(lv[end+1]))) {
			continue
		}
		out = append(out, [2]int{start, end})
	}
	return out
}

// ReferencesHost reports whether v contains host as a whole hostname token.
func ReferencesHost(v, host string) bool { return len(hostSpans(v, host)) > 0 }

// RewriteValue replaces every whole-token occurrence of each mapping's
// source host and returns the new value with the number of replacements.
func RewriteValue(v string, maps []Mapping) (string, int) {
	total := 0
	for _, m := range maps {
		spans := hostSpans(v, m.From)
		if len(spans) == 0 {
			continue
		}
		var b strings.Builder
		prev := 0
		for _, s := range spans {
			b.WriteString(v[prev:s[0]])
			b.WriteString(m.To)
			prev = s[1]
		}
		b.WriteString(v[prev:])
		v = b.String()
		total += len(spans)
	}
	return v, total
}

// Change is one env value a mapping rewrote, in a form safe to show.
type Change struct {
	App    string `json:"app"`
	Key    string `json:"key"`
	Secret bool   `json:"secret"`
	Count  int    `json:"count"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// RewriteEnv applies maps to env values and returns the new env with a
// masked diff. The input slice is not modified.
func RewriteEnv(app string, env []platformimport.Env, maps []Mapping) ([]platformimport.Env, []Change) {
	out := make([]platformimport.Env, len(env))
	copy(out, env)
	var changes []Change
	for i, e := range out {
		nv, n := RewriteValue(e.Value, maps)
		if n == 0 {
			continue
		}
		out[i].Value = nv
		ch := Change{App: app, Key: e.Key, Secret: e.Secret, Count: n}
		ch.Before, ch.After = maskPair(e.Value, nv, e.Secret, maps)
		changes = append(changes, ch)
	}
	return out, changes
}

func maskPair(before, after string, secret bool, maps []Mapping) (string, string) {
	mb, ma := maskURLPassword(before), maskURLPassword(after)
	if !secret || strings.Contains(before, "://") {
		return mb, ma
	}
	for _, m := range maps {
		if ReferencesHost(before, m.From) {
			return "..." + m.From + "...", "..." + m.To + "..."
		}
	}
	return "...", "..."
}

// maskURLPassword hides the password of a URL in v, or of every URL found
// inside a longer value.
func maskURLPassword(v string) string {
	if !strings.Contains(v, "://") {
		return v
	}
	if u, err := url.Parse(v); err == nil && u.User != nil {
		if _, has := u.User.Password(); has {
			u.User = url.UserPassword(u.User.Username(), "****")
			return strings.Replace(u.String(), "%2A%2A%2A%2A", "****", 1)
		}
	}
	return urlPasswordRe.ReplaceAllString(v, "${1}****@")
}

var urlPasswordRe = regexp.MustCompile(`(://[^:/@\s]*:)[^@\s/]*@`)

// FindReferences lists which of hosts appear in env values, so a caller can
// tell which databases an app depends on. Values are inspected, not kept.
func FindReferences(env []platformimport.Env, hosts []string) map[string]bool {
	found := map[string]bool{}
	for _, e := range env {
		for _, h := range hosts {
			if !found[h] && ReferencesHost(e.Value, h) {
				found[h] = true
			}
		}
	}
	return found
}
