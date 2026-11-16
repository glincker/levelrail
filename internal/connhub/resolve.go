package connhub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Binding roles and the connection capabilities that can fill each one.
var roleCapabilities = map[string][]string{
	"source":        {"source"},
	"deploy_report": {"status", "deployments"},
	"registry_pull": {"registry_pull"},
	"notify":        {"notify"},
	"secrets":       {"secrets"},
	"dns":           {"dns"},
}

// Roles lists every binding role in a stable order.
func Roles() []string {
	out := make([]string, 0, len(roleCapabilities))
	for r := range roleCapabilities {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// KnownRole reports whether role is a binding role.
func KnownRole(role string) bool {
	_, ok := roleCapabilities[role]
	return ok
}

// RoleSupported reports whether c has a capability that can fill role.
func RoleSupported(c Connection, role string) bool {
	for _, want := range roleCapabilities[role] {
		if slices.Contains(c.Capabilities, want) {
			return true
		}
	}
	return false
}

type ruleKind int

const (
	ruleString ruleKind = iota
	ruleBool
	ruleStringList
)

type ruleField struct {
	kind ruleKind
	enum []string
}

// ruleSchemas is the allowed rules shape per role; unknown keys are refused so
// a typo never silently turns into a default.
var ruleSchemas = map[string]map[string]ruleField{
	"source": {
		"branch":      {kind: ruleString},
		"trigger":     {kind: ruleString, enum: []string{"push", "tag", "release"}},
		"paths":       {kind: ruleStringList},
		"previews":    {kind: ruleBool},
		"comments":    {kind: ruleBool},
		"wait_for_ci": {kind: ruleBool},
	},
	"deploy_report": {
		"statuses":    {kind: ruleBool},
		"deployments": {kind: ruleBool},
		"environment": {kind: ruleString},
	},
	"registry_pull": {"host": {kind: ruleString}},
	"notify":        {"events": {kind: ruleStringList, enum: []string{"deploy_failed", "preview_ready", "expiry_warning"}}},
	"secrets":       {},
	"dns":           {},
}

// DefaultRuleValueMaxLen bounds one rule string when no limit is configured.
const DefaultRuleValueMaxLen = 512

// RuleLimits caps the whole rules document and each string inside it.
type RuleLimits struct {
	MaxBytes    int
	MaxValueLen int
}

// ValidateRules checks raw against the role's schema and the size caps, and
// returns the canonical JSON to store. Empty input means no rules.
func ValidateRules(role string, raw []byte, lim RuleLimits) (string, error) {
	maxBytes := lim.MaxBytes
	if lim.MaxValueLen <= 0 {
		lim.MaxValueLen = DefaultRuleValueMaxLen
	}
	schema, ok := ruleSchemas[role]
	if !ok {
		return "", fmt.Errorf("%w: unknown role %q", ErrInvalid, role)
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "{}", nil
	}
	if maxBytes > 0 && len(raw) > maxBytes {
		return "", fmt.Errorf("%w: rules are larger than %d bytes", ErrInvalid, maxBytes)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", fmt.Errorf("%w: rules must be a JSON object", ErrInvalid)
	}
	out := make(map[string]any, len(obj))
	for key, val := range obj {
		f, ok := schema[key]
		if !ok {
			return "", fmt.Errorf("%w: rule %q is not valid for role %s", ErrInvalid, key, role)
		}
		v, err := f.decode(key, val, lim.MaxValueLen)
		if err != nil {
			return "", err
		}
		out[key] = v
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("connhub: encode rules: %w", err)
	}
	return string(b), nil
}

func (f ruleField) decode(key string, val json.RawMessage, maxLen int) (any, error) {
	switch f.kind {
	case ruleBool:
		var b bool
		if err := json.Unmarshal(val, &b); err != nil {
			return nil, fmt.Errorf("%w: rule %q must be true or false", ErrInvalid, key)
		}
		return b, nil
	case ruleStringList:
		var list []string
		if err := json.Unmarshal(val, &list); err != nil {
			return nil, fmt.Errorf("%w: rule %q must be a list of strings", ErrInvalid, key)
		}
		for _, s := range list {
			if err := checkRuleString(key, s, maxLen); err != nil {
				return nil, err
			}
			if err := f.checkEnum(key, s); err != nil {
				return nil, err
			}
		}
		if list == nil {
			list = []string{}
		}
		return list, nil
	default:
		var s string
		if err := json.Unmarshal(val, &s); err != nil {
			return nil, fmt.Errorf("%w: rule %q must be a string", ErrInvalid, key)
		}
		s = strings.TrimSpace(s)
		if err := checkRuleString(key, s, maxLen); err != nil {
			return nil, err
		}
		return s, f.checkEnum(key, s)
	}
}

// checkRuleString keeps rule strings to one line of printable text within
// maxLen bytes, since branch names and path globs reach git and glob code.
func checkRuleString(key, s string, maxLen int) error {
	if len(s) > maxLen {
		return fmt.Errorf("%w: rule %q values must be at most %d bytes", ErrInvalid, key, maxLen)
	}
	if !utf8.ValidString(s) {
		return fmt.Errorf("%w: rule %q must be valid UTF-8", ErrInvalid, key)
	}
	for _, r := range s {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return fmt.Errorf("%w: rule %q must not contain control characters or line breaks", ErrInvalid, key)
		}
	}
	return nil
}

func (f ruleField) checkEnum(key, v string) error {
	if len(f.enum) == 0 || slices.Contains(f.enum, v) {
		return nil
	}
	return fmt.Errorf("%w: rule %q must be one of %s", ErrInvalid, key, strings.Join(f.enum, ", "))
}

// Target is what a binding points at. TeamID is the org that owns an app
// target, or the org itself for an org target.
type Target struct {
	Type   string
	ID     string
	TeamID string
}

// Covers reports whether a connection's scope reaches target. A binding is
// refused when it does not, so binding never widens a connection's reach.
func Covers(c Connection, t Target) bool {
	switch c.ScopeType {
	case "", "instance":
		return true
	case "org":
		return c.ScopeID != "" && c.ScopeID == t.TeamID
	case "app":
		return t.Type == "app" && c.ScopeID != "" && c.ScopeID == t.ID
	}
	return false
}

// ResolvedBinding is one effective binding for an app. InheritedFrom names the
// org when the row comes from org defaults; Overridden marks an org row an app
// binding replaces for the same role.
type ResolvedBinding struct {
	Binding
	InheritedFrom string `json:"inherited_from,omitempty"`
	Overridden    bool   `json:"overridden,omitempty"`
}

// Resolve merges an app's own bindings with its org's: per role the app wins,
// the org fills gaps. Stored rows whose connection is gone or whose scope no
// longer covers the app are dropped, so a later scope change cannot leak.
func Resolve(app, org []Binding, conns map[string]Connection, target Target) []ResolvedBinding {
	out := []ResolvedBinding{}
	appRoles := map[string]bool{}
	for _, b := range app {
		if b.Origin == "binding" && !inScope(b, conns, target) {
			continue
		}
		appRoles[b.Role] = true
		out = append(out, ResolvedBinding{Binding: b})
	}
	for _, b := range org {
		if !inScope(b, conns, target) {
			continue
		}
		b.Origin = "inherited"
		out = append(out, ResolvedBinding{Binding: b, InheritedFrom: b.TargetID, Overridden: appRoles[b.Role]})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role < out[j].Role
		}
		return !out[i].Overridden && out[j].Overridden
	})
	return out
}

// CoveredBindings keeps the rows Resolve would keep from an app's own list:
// derived rows, and stored rows whose connection still covers the target.
func CoveredBindings(rows []Binding, conns map[string]Connection, target Target) []Binding {
	out := make([]Binding, 0, len(rows))
	for _, b := range rows {
		if b.Origin == "binding" && !inScope(b, conns, target) {
			continue
		}
		out = append(out, b)
	}
	return out
}

func inScope(b Binding, conns map[string]Connection, target Target) bool {
	c, ok := conns[b.Ref]
	return ok && Covers(c, target)
}
