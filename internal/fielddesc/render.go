package fielddesc

import (
	"fmt"
	"strings"
)

// FlagSpec is one CLI flag derived from the descriptors.
type FlagSpec struct {
	Name  string
	Key   string
	Usage string
	File  bool
}

// FlagSpecs lists each distinct flag once, in field order. Variants that share a wire
// key share one flag, so the first descriptor carrying the flag supplies its help.
func (r Resource) FlagSpecs() []FlagSpec {
	seen := map[string]bool{}
	var out []FlagSpec
	for _, f := range r.Fields {
		if f.Flag == "" || seen[f.Flag] {
			continue
		}
		seen[f.Flag] = true
		out = append(out, FlagSpec{Name: f.Flag, Key: f.Key, Usage: f.Doc, File: f.FlagFile})
	}
	return out
}

// Markdown renders the docs table for a resource.
func (r Resource) Markdown() string {
	var b strings.Builder
	b.WriteString("| Field | Type | Required | Shown when | CLI flag | Description |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, f := range r.Fields {
		flag := "none"
		if f.Flag != "" {
			flag = "`--" + f.Flag + "`"
		}
		req := "no"
		if f.Required {
			req = "yes"
		}
		typ := string(f.Type)
		if f.Secret {
			typ += ", write only"
		}
		fmt.Fprintf(&b, "| `%s` (%s) | %s | %s | %s | %s | %s |\n", f.Key, f.ID, typ, req, whenText(f.ShowWhen), flag, f.Doc)
	}
	return b.String()
}

func whenText(cs []Condition) string {
	if len(cs) == 0 {
		return "always"
	}
	parts := make([]string, 0, len(cs))
	for _, c := range cs {
		parts = append(parts, fmt.Sprintf("`%s` is %s", c.Field, strings.Join(c.Equals, " or ")))
	}
	return strings.Join(parts, " and ")
}
