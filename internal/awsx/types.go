package awsx

import (
	"bytes"
	"encoding/json"
	"sort"
	"time"
)

// Secret holds a value that must never reach logs, errors or JSON output.
type Secret string

// Reveal returns the raw value for the one place that needs it.
func (s Secret) Reveal() string { return string(s) }

// String implements fmt.Stringer.
func (Secret) String() string { return "[redacted]" }

// GoString keeps %#v from printing the value.
func (Secret) GoString() string { return "[redacted]" }

// MarshalJSON writes a placeholder, never the value.
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`"[redacted]"`), nil }

// UnmarshalJSON reads a plain JSON string into the secret.
func (s *Secret) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = Secret(v)
	return nil
}

// EpochTime decodes the JSON 1.1 timestamp, seconds since epoch as a number.
type EpochTime struct{ time.Time }

// UnmarshalJSON accepts a number or null.
func (t *EpochTime) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	sec := int64(f)
	t.Time = time.Unix(sec, int64((f-float64(sec))*1e9)).UTC()
	return nil
}

// Tag is one AWS key and value pair.
type Tag struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

// TagList converts a map to a stable list.
func TagList(m map[string]string) []Tag {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Tag, 0, len(keys))
	for _, k := range keys {
		out = append(out, Tag{Key: k, Value: m[k]})
	}
	return out
}
