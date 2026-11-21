// Package fielddesc is the one registry of resource form fields. The API serves it,
// the CLI builds flags from it, the dashboard renders forms from it and the docs
// table is generated from it, so adding a field is one entry here.
package fielddesc

import (
	"fmt"
	"sort"
	"strings"
)

// Type is the input kind a surface renders for a field.
type Type string

// Input kinds a surface knows how to render.
const (
	TypeString Type = "string"
	TypeText   Type = "text"
	TypeSecret Type = "secret"
	TypeSelect Type = "select"
	TypeBool   Type = "bool"
)

// Condition shows a field only while another field holds one of Equals.
type Condition struct {
	Field  string   `json:"field"`
	Equals []string `json:"equals"`
}

// Field describes one input of a resource form. ID is unique per resource; Key is the
// wire name and may repeat across disjoint ShowWhen variants (for example one key
// labelled per provider).
type Field struct {
	ID       string      `json:"id"`
	Key      string      `json:"key"`
	Type     Type        `json:"type"`
	LabelKey string      `json:"label_key"`
	HelpKey  string      `json:"help_key"`
	Default  string      `json:"default,omitempty"`
	Options  []string    `json:"options,omitempty"`
	Required bool        `json:"required"`
	Secret   bool        `json:"secret"`
	ShowWhen []Condition `json:"show_when,omitempty"`
	// Flag is the CLI flag name without dashes; empty means the field has no flag.
	Flag string `json:"flag,omitempty"`
	// FlagFile marks a flag that takes a file path whose content becomes the value.
	FlagFile bool `json:"flag_file,omitempty"`
	// FormOnly marks a field the CLI infers instead of taking a flag.
	FormOnly bool `json:"form_only,omitempty"`
	// Placeholder and HelpURL are language neutral hints the dashboard shows beside the input.
	Placeholder string `json:"placeholder,omitempty"`
	HelpURL     string `json:"help_url,omitempty"`
	// Doc is the English one line description used by the CLI help and the docs table.
	Doc string `json:"doc"`
}

// Docs places the generated docs page of a resource. It is not served over the API.
type Docs struct {
	File        string // path under docs/, for example backup-target-fields.md
	Title       string
	Description string
}

// Resource is the ordered field list of one resource kind.
type Resource struct {
	Name   string  `json:"name"`
	Fields []Field `json:"fields"`
	// Docs is required: the generator and the contract test read it for every resource.
	Docs Docs `json:"-"`
}

var registry = map[string]Resource{}

// Register adds a resource at init time; a duplicate name or field ID is a programming error.
func Register(r Resource) {
	if _, dup := registry[r.Name]; dup {
		panic("fielddesc: duplicate resource " + r.Name)
	}
	if r.Docs.File == "" || r.Docs.Title == "" {
		panic("fielddesc: resource " + r.Name + " needs Docs.File and Docs.Title")
	}
	seen := map[string]bool{}
	for _, f := range r.Fields {
		if seen[f.ID] {
			panic("fielddesc: duplicate field id " + r.Name + "." + f.ID)
		}
		seen[f.ID] = true
	}
	registry[r.Name] = r
}

// Get returns a resource by name.
func Get(name string) (Resource, bool) {
	r, ok := registry[name]
	return r, ok
}

// Names lists registered resources, sorted.
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Visible reports whether f applies given the current form values.
func (f Field) Visible(values map[string]string) bool {
	for _, c := range f.ShowWhen {
		ok := false
		for _, v := range c.Equals {
			if values[c.Field] == v {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// Problem is one validation failure, worded identically for every surface.
type Problem struct {
	Key     string `json:"key"`
	Message string `json:"message"`
}

// Validate checks visible fields for required values and select options.
func (r Resource) Validate(values map[string]string) []Problem {
	return r.ValidateExcept(values)
}

// ValidateExcept is Validate without the named keys, for updates where blank credentials
// mean "keep what is stored".
func (r Resource) ValidateExcept(values map[string]string, skip ...string) []Problem {
	var out []Problem
	for _, f := range r.Fields {
		if !f.Visible(values) || contains(skip, f.Key) {
			continue
		}
		v := strings.TrimSpace(values[f.Key])
		if f.Required && v == "" {
			out = append(out, Problem{Key: f.Key, Message: fmt.Sprintf("%s is required", f.Key)})
			continue
		}
		if v != "" && f.Type == TypeSelect && !contains(f.Options, v) {
			out = append(out, Problem{Key: f.Key, Message: fmt.Sprintf("%s must be one of %s", f.Key, strings.Join(f.Options, ", "))})
		}
	}
	return out
}

// Masked returns a copy of values with every visible secret replaced by a fixed mask, so a
// read path can never echo a credential.
func (r Resource) Masked(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		out[k] = v
	}
	for _, f := range r.Fields {
		if f.Secret {
			if _, ok := out[f.Key]; ok && out[f.Key] != "" {
				out[f.Key] = SecretMask
			}
		}
	}
	return out
}

// SecretMask is what a read returns in place of a stored secret.
const SecretMask = "********"

// FirstProblem returns the first problem as an error text, or empty when valid.
func FirstProblem(ps []Problem) string {
	if len(ps) == 0 {
		return ""
	}
	return ps[0].Message
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
