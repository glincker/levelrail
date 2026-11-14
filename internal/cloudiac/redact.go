package cloudiac

import (
	"encoding/json"
	"strings"
)

// minSensitiveLen skips short values so common words are not masked everywhere.
const minSensitiveLen = 6

// SensitiveValues collects every string under an attribute that plan or state JSON marks sensitive.
func SensitiveValues(raws ...[]byte) []string {
	seen := map[string]bool{}
	var out []string
	add := func(v any) {
		walkStrings(v, func(s string) {
			if len(s) >= minSensitiveLen && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		})
	}
	for _, raw := range raws {
		if len(raw) == 0 {
			continue
		}
		var plan planJSON
		if json.Unmarshal(raw, &plan) == nil {
			for _, rc := range plan.ResourceChanges {
				for k := range sensitiveKeys(rc.Change.BeforeSensitive) {
					add(rc.Change.Before[k])
				}
				for k := range sensitiveKeys(rc.Change.AfterSensitive) {
					add(rc.Change.After[k])
				}
			}
		}
		var st stateJSON
		if json.Unmarshal(raw, &st) == nil {
			for _, r := range st.Resources {
				for _, in := range r.Instances {
					for _, k := range sensitiveStateKeys(in.SensitiveAttributes) {
						add(in.Attributes[k])
					}
				}
			}
			for _, o := range st.Outputs {
				if o.Sensitive {
					add(o.Value)
				}
			}
		}
	}
	return out
}

type stateJSON struct {
	Outputs map[string]struct {
		Value     any  `json:"value"`
		Sensitive bool `json:"sensitive"`
	} `json:"outputs"`
	Resources []struct {
		Instances []struct {
			Attributes          map[string]any `json:"attributes"`
			SensitiveAttributes []any          `json:"sensitive_attributes"`
		} `json:"instances"`
	} `json:"resources"`
}

// sensitiveStateKeys reads the top level attribute of each sensitive path, in either state path format.
func sensitiveStateKeys(paths []any) []string {
	var out []string
	for _, p := range paths {
		switch t := p.(type) {
		case []any:
			if len(t) > 0 {
				if step, ok := t[0].(map[string]any); ok {
					if v, ok := step["value"].(string); ok {
						out = append(out, v)
					}
				}
			}
		case map[string]any:
			if v, ok := t["value"].(string); ok {
				out = append(out, v)
			}
		}
	}
	return out
}

func walkStrings(v any, fn func(string)) {
	switch t := v.(type) {
	case string:
		fn(t)
	case map[string]any:
		for _, x := range t {
			walkStrings(x, fn)
		}
	case []any:
		for _, x := range t {
			walkStrings(x, fn)
		}
	}
}

// RedactStepOutput masks credential lines, the whole state blob and each sensitive value,
// then keeps the tail within limit. Masking runs first so a cut never exposes half a secret.
func RedactStepOutput(out string, limit int, credRedact func(string) string, sensitive []string, state []byte) string {
	if credRedact != nil {
		out = credRedact(out)
	}
	out = RedactLog(out, append([]string{string(state)}, sensitive...)...)
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return strings.TrimSpace(out)
}
