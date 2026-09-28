package iac

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Resource is one desired object in canonical form, ready to compare with
// live state.
type Resource struct {
	Kind   Kind
	Name   string
	Scope  string
	Doc    *Document
	Fields map[string]any
	// SecretRefs are app env names backed by the secrets store.
	SecretRefs []string
	Warnings   []string
}

// Key is the stable identity used in plans: kind plus scope plus name.
func (r *Resource) Key() string { return resourceKey(r.Kind, r.Scope, r.Name) }

func resourceKey(kind Kind, scope, name string) string {
	if scope != "" {
		return string(kind) + "/" + scope + "/" + name
	}
	return string(kind) + "/" + name
}

func toMap(v any) map[string]any {
	raw, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func sortedUnique(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// fmtDuration renders d in the shortest whole unit the schema's duration
// pattern accepts, so parse and format round trip.
func fmtDuration(d time.Duration) string {
	switch {
	case d <= 0:
		return ""
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	case d%time.Second == 0:
		return fmt.Sprintf("%ds", d/time.Second)
	default:
		return fmt.Sprintf("%dms", d/time.Millisecond)
	}
}

func normDuration(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return "", fmt.Errorf("invalid duration %q", s)
	}
	return fmtDuration(d), nil
}

var (
	secretKeyRe   = regexp.MustCompile(`(?i)(secret|token|passw|api_?key|private_?key|credential|access_?key)`)
	secretValueRe = regexp.MustCompile(`(?i)^(-----BEGIN [A-Z ]*PRIVATE KEY|ghp_[A-Za-z0-9]{20,}|gho_[A-Za-z0-9]{20,}|github_pat_|sk_live_|sk-[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|xox[baprs]-|glpat-)`)
	secretPlaceRe = regexp.MustCompile(`^\s*\$\{\{\s*secrets\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}\s*$`)
	envPlaceRe    = regexp.MustCompile(`\$\{\{\s*env\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
)

// looksSecret reports whether a literal env value must not live in a file.
func looksSecret(key, value string) bool {
	switch strings.ToLower(value) {
	case "", "true", "false", "0", "1", "yes", "no":
		return false
	}
	return secretKeyRe.MatchString(key) || secretValueRe.MatchString(value)
}

// substituteVars replaces ${{ env.NAME }} with vars[NAME] and returns the
// names it could not resolve.
func substituteVars(s string, vars map[string]string) (string, []string) {
	var missing []string
	out := envPlaceRe.ReplaceAllStringFunc(s, func(m string) string {
		name := envPlaceRe.FindStringSubmatch(m)[1]
		v, ok := vars[name]
		if !ok {
			missing = append(missing, name)
			return m
		}
		return v
	})
	return out, missing
}

func stringList(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if str, ok := e.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func remarshal(in, out any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
