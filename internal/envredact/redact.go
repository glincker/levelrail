package envredact

import (
	"fmt"
	"strings"
)

// Meta describes one variable on a read surface without its value.
type Meta struct {
	Sensitive     bool   `json:"sensitive"`
	Set           bool   `json:"set"`
	ValueRedacted bool   `json:"value_redacted"`
	Reason        string `json:"reason,omitempty"`
	Override      *bool  `json:"override,omitempty"`
}

// PlaceholderError names a key that was sent as Placeholder but has no
// stored value to keep.
type PlaceholderError struct{ Key string }

func (e *PlaceholderError) Error() string {
	return fmt.Sprintf("%s holds the masked placeholder but has no stored value to keep: send a real value", e.Key)
}

// IsPlaceholder reports whether v is the masked placeholder.
func IsPlaceholder(v string) bool {
	return strings.TrimSpace(v) == Placeholder
}

func overrideFor(overrides map[string]bool, key string) *bool {
	if v, ok := overrides[key]; ok {
		return &v
	}
	return nil
}

// Value returns what a read surface may show for key=value and its meta.
func Value(key, value string, overrides map[string]bool) (string, Meta) {
	ov := overrideFor(overrides, key)
	sensitive, reason := Classify(key, value, ov)
	m := Meta{Sensitive: sensitive, Set: value != "", Reason: reason, Override: ov}
	if !sensitive || value == "" {
		return value, m
	}
	m.ValueRedacted = true
	return Placeholder, m
}

// Map masks every sensitive value in env. The input map is never mutated.
func Map(env map[string]string, overrides map[string]bool) (map[string]string, map[string]Meta) {
	if env == nil {
		return nil, nil
	}
	out := make(map[string]string, len(env))
	meta := make(map[string]Meta, len(env))
	for k, v := range env {
		out[k], meta[k] = Value(k, v, overrides)
	}
	return out, meta
}

// Merge resolves a submitted env against the stored one: a Placeholder
// keeps the stored value, and a Placeholder with nothing stored fails.
func Merge(submitted, stored map[string]string) (map[string]string, error) {
	if submitted == nil {
		return nil, nil
	}
	out := make(map[string]string, len(submitted))
	for k, v := range submitted {
		if !IsPlaceholder(v) {
			out[k] = v
			continue
		}
		prev, ok := stored[k]
		if !ok {
			return nil, &PlaceholderError{Key: k}
		}
		out[k] = prev
	}
	return out, nil
}
