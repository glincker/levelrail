// Package configtranslate reads hosted-platform config files (Heroku
// Procfile and app.json, Vercel vercel.json) and proposes a Levelrail app
// spec fragment plus findings for what has no equivalent. It is pure and
// offline: same input, same output, nothing is read from disk or network.
package configtranslate

import (
	"bytes"
	"fmt"
	"path"
	"sort"

	"gopkg.in/yaml.v3"
)

// Format names a supported source config format.
type Format string

// Supported source formats.
const (
	FormatHeroku Format = "heroku"
	FormatVercel Format = "vercel"
)

// Severity ranks how much a finding needs the operator's attention.
type Severity string

// Finding severities: info is FYI, warn changes behavior, blocker has no equivalent.
const (
	SeverityInfo    Severity = "info"
	SeverityWarn    Severity = "warn"
	SeverityBlocker Severity = "blocker"
)

// Finding is one thing the operator should know or do, with a stable code.
type Finding struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	// Source is the input file the finding came from.
	Source string `json:"source"`
	// Subject names the setting concerned (an env key, add-on, route).
	Subject string `json:"subject,omitempty"`
	Message string `json:"message"`
	// Fix is a short human instruction.
	Fix string `json:"fix"`
}

// Result is the full output of a translation.
type Result struct {
	Detected []Format  `json:"detected"`
	Fragment Fragment  `json:"fragment"`
	Findings []Finding `json:"findings"`
}

// Default ports proposed when the source config names none.
const (
	DefaultWebPort  = 8080
	DefaultNodePort = 3000
)

type options struct{ webPort, nodePort int }

// Option tunes a translation without making the package read the environment.
type Option func(*options)

// WithWebPort sets the port proposed for Heroku web and worker services.
func WithWebPort(p int) Option {
	return func(o *options) {
		if p > 0 {
			o.webPort = p
		}
	}
}

// WithNodePort sets the port proposed for a Vercel server service.
func WithNodePort(p int) Option {
	return func(o *options) {
		if p > 0 {
			o.nodePort = p
		}
	}
}

// Translate detects the known config files in files (keyed by repo-relative
// path) and returns the merged proposal. Unknown files are ignored.
func Translate(files map[string][]byte, opts ...Option) Result {
	o := options{webPort: DefaultWebPort, nodePort: DefaultNodePort}
	for _, opt := range opts {
		opt(&o)
	}
	res := Result{Fragment: Fragment{Version: 1}}
	res.Detected = Detect(files)
	for _, f := range res.Detected {
		var part Fragment
		var fs []Finding
		switch f {
		case FormatHeroku:
			part, fs = translateHeroku(files, o)
		case FormatVercel:
			part, fs = translateVercel(files, o)
		}
		res.Findings = append(res.Findings, fs...)
		res.Findings = append(res.Findings, res.Fragment.merge(part)...)
	}
	return res
}

// Detect reports which formats have at least one input file present, in a
// fixed order.
func Detect(files map[string][]byte) []Format {
	var out []Format
	if has(files, "Procfile") || has(files, "app.json") {
		out = append(out, FormatHeroku)
	}
	if has(files, "vercel.json") {
		out = append(out, FormatVercel)
	}
	return out
}

func has(files map[string][]byte, name string) bool {
	_, ok := lookup(files, name)
	return ok
}

// lookup finds a root-level file, tolerating "./" and cleaned prefixes.
func lookup(files map[string][]byte, name string) ([]byte, bool) {
	if b, ok := files[name]; ok {
		return b, true
	}
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if path.Clean(k) == name {
			return files[k], true
		}
	}
	return nil, false
}

// YAML renders the fragment as app.yaml text with sorted, stable keys.
func (f Fragment) YAML() ([]byte, error) {
	var root yaml.Node
	if err := root.Encode(f); err != nil {
		return nil, fmt.Errorf("configtranslate: encode fragment: %w", err)
	}
	if err := attachEnv(&root, f); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return nil, fmt.Errorf("configtranslate: marshal fragment: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("configtranslate: marshal fragment: %w", err)
	}
	return buf.Bytes(), nil
}

// attachEnv appends each service's env block, which has a string-or-object
// shape the yaml encoder cannot express through struct tags alone.
func attachEnv(root *yaml.Node, f Fragment) error {
	svcs := mapValue(root, "services")
	if svcs == nil {
		return nil
	}
	for i := 0; i+1 < len(svcs.Content); i += 2 {
		name := svcs.Content[i].Value
		env := f.Services[name].Env
		if len(env) == 0 {
			continue
		}
		block := &yaml.Node{Kind: yaml.MappingNode}
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v, err := env[k].node()
			if err != nil {
				return err
			}
			block.Content = append(block.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, v)
		}
		m := svcs.Content[i+1]
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "env"}, block)
	}
	return nil
}

func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}
