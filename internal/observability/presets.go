// Package observability turns a short provider form into data export
// destinations. Provider facts live in presets.json, this code only renders.
package observability

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/glincker/levelrail/internal/dataexport"
	"github.com/glincker/levelrail/internal/store"
)

//go:embed presets.json
var presetsJSON []byte

// Choice is one allowed value of a select field. Vars are extra template
// variables that apply when it is picked, such as a regional host.
type Choice struct {
	Value string            `json:"value"`
	Label string            `json:"label"`
	Vars  map[string]string `json:"vars,omitempty"`
}

// Field is one input of a preset form. Secret fields are never echoed back.
type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type,omitempty"`
	Secret      bool     `json:"secret,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Default     string   `json:"default,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Help        string   `json:"help,omitempty"`
	Choices     []Choice `json:"choices,omitempty"`
}

// SignalSpec describes how one signal is delivered for a preset.
type SignalSpec struct {
	Signal   string            `json:"signal"`
	Kind     store.ExportKind  `json:"kind"`
	Default  bool              `json:"default"`
	Endpoint string            `json:"endpoint,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Options  map[string]string `json:"options,omitempty"`
	APIKey   string            `json:"api_key,omitempty"`
}

// Preset is one provider entry of the catalog.
type Preset struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	DocsAnchor  string            `json:"docs_anchor"`
	Fields      []Field           `json:"fields"`
	Signals     []SignalSpec      `json:"signals"`
	Hints       map[string]string `json:"hints"`
}

var catalog = mustLoad()

func mustLoad() []Preset {
	var f struct {
		Presets []Preset `json:"presets"`
	}
	if err := json.Unmarshal(presetsJSON, &f); err != nil {
		panic("observability: presets.json: " + err.Error())
	}
	return f.Presets
}

// Catalog returns every preset in display order.
func Catalog() []Preset { return catalog }

// Lookup finds a preset by id.
func Lookup(id string) (Preset, bool) {
	for _, p := range catalog {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// ErrInvalid wraps every form validation failure so callers can answer 400.
var ErrInvalid = fmt.Errorf("invalid observability export")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Rendered is one destination ready to create. Endpoint and HeaderNames are
// safe to show; Input carries the credential and must not be logged.
type Rendered struct {
	Signal      string
	Input       dataexport.Input
	Endpoint    string
	HeaderNames []string
}

var tokenRe = regexp.MustCompile(`\{([a-z_]+(?::[a-z_]+)*)\}`)

// SignalNames lists the signals a preset can export.
func (p Preset) SignalNames() []string {
	out := make([]string, 0, len(p.Signals))
	for _, s := range p.Signals {
		out = append(out, s.Signal)
	}
	return out
}

// DefaultSignals lists the signals exported when the caller names none.
func (p Preset) DefaultSignals() []string {
	var out []string
	for _, s := range p.Signals {
		if s.Default {
			out = append(out, s.Signal)
		}
	}
	return out
}

func (p Preset) vars(fields map[string]string) (map[string]string, error) {
	known := map[string]Field{}
	for _, f := range p.Fields {
		known[f.Key] = f
	}
	for k := range fields {
		if _, ok := known[k]; !ok {
			return nil, invalid("unknown field %q for preset %s", k, p.ID)
		}
	}
	vars := map[string]string{}
	for _, f := range p.Fields {
		v := strings.TrimSpace(fields[f.Key])
		if v == "" {
			v = f.Default
		}
		if v == "" {
			if f.Required {
				return nil, invalid("field %q is required", f.Key)
			}
			vars[f.Key] = ""
			continue
		}
		if len(v) > 2048 || strings.ContainsAny(v, "\r\n\x00") || (!f.Secret && strings.Contains(v, " ")) {
			return nil, invalid("field %q has an invalid value", f.Key)
		}
		if f.Type == "url" {
			v = strings.TrimRight(v, "/")
		}
		if len(f.Choices) > 0 {
			var hit *Choice
			for i := range f.Choices {
				if f.Choices[i].Value == v {
					hit = &f.Choices[i]
				}
			}
			if hit == nil {
				return nil, invalid("field %q must be one of %s", f.Key, choiceValues(f.Choices))
			}
			for k, cv := range hit.Vars {
				vars[k] = cv
			}
		}
		vars[f.Key] = v
	}
	return vars, nil
}

func choiceValues(cs []Choice) string {
	vals := make([]string, len(cs))
	for i, c := range cs {
		vals[i] = c.Value
	}
	return strings.Join(vals, ", ")
}

func expand(tpl string, vars map[string]string) (string, error) {
	var bad error
	out := tokenRe.ReplaceAllStringFunc(tpl, func(m string) string {
		parts := strings.Split(m[1:len(m)-1], ":")
		if parts[0] == "basic" && len(parts) == 3 {
			return base64.StdEncoding.EncodeToString([]byte(vars[parts[1]] + ":" + vars[parts[2]]))
		}
		if parts[0] == "authbasic" && len(parts) == 3 {
			// Blank user and token mean no Authorization header at all.
			if vars[parts[1]] == "" && vars[parts[2]] == "" {
				return ""
			}
			return "Basic " + base64.StdEncoding.EncodeToString([]byte(vars[parts[1]]+":"+vars[parts[2]]))
		}
		if len(parts) != 1 {
			bad = fmt.Errorf("unknown template function %q", m)
			return ""
		}
		v, ok := vars[parts[0]]
		if !ok {
			bad = fmt.Errorf("unknown template variable %q", m)
		}
		return v
	})
	return out, bad
}

// Render builds the destinations for the chosen signals. An empty signal
// list means the preset defaults. name groups the destinations of one export.
func (p Preset) Render(name string, fields map[string]string, signals []string) ([]Rendered, error) {
	vars, err := p.vars(fields)
	if err != nil {
		return nil, err
	}
	if len(signals) == 0 {
		signals = p.DefaultSignals()
	}
	seen := map[string]bool{}
	var out []Rendered
	for _, want := range signals {
		if seen[want] {
			continue
		}
		seen[want] = true
		spec, ok := p.spec(want)
		if !ok {
			return nil, invalid("preset %s cannot export %q, choose from %s", p.ID, want, strings.Join(p.SignalNames(), ", "))
		}
		r, err := p.renderSignal(name, spec, vars)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, invalid("choose at least one signal")
	}
	return out, nil
}

func (p Preset) spec(signal string) (SignalSpec, bool) {
	for _, s := range p.Signals {
		if s.Signal == signal {
			return s, true
		}
	}
	return SignalSpec{}, false
}

func (p Preset) renderSignal(name string, s SignalSpec, vars map[string]string) (Rendered, error) {
	in := dataexport.Input{Name: DestinationName(name, p.ID, s.Signal), Kind: s.Kind, Enabled: true}
	var err error
	if s.Endpoint != "" {
		if in.Endpoint, err = expand(s.Endpoint, vars); err != nil {
			return Rendered{}, err
		}
	}
	if len(s.Headers) > 0 {
		in.Headers = map[string]string{}
	}
	for k, v := range s.Headers {
		hk, err := expand(k, vars)
		if err != nil {
			return Rendered{}, err
		}
		hv, err := expand(v, vars)
		if err != nil {
			return Rendered{}, err
		}
		// An optional credential left blank means no header at all.
		if hk != "" && hv != "" {
			in.Headers[hk] = hv
		}
	}
	for k, v := range s.Options {
		ov, err := expand(v, vars)
		if err != nil {
			return Rendered{}, err
		}
		applyOption(&in.Options, k, ov)
	}
	if s.APIKey != "" {
		if in.APIKey, err = expand(s.APIKey, vars); err != nil {
			return Rendered{}, err
		}
	}
	shown := in.Endpoint
	if shown == "" {
		if shown, err = dataexport.VendorEndpoint(in.Kind, in.Options); err != nil {
			return Rendered{}, invalid("%v", err)
		}
	}
	names := make([]string, 0, len(in.Headers))
	for k := range in.Headers {
		names = append(names, k)
	}
	sort.Strings(names)
	return Rendered{Signal: s.Signal, Input: in, Endpoint: shown, HeaderNames: names}, nil
}

func applyOption(o *store.ExportOptions, key, val string) {
	switch key {
	case "site":
		o.Site = val
	case "dataset":
		o.Dataset = val
	case "region":
		o.Region = val
	}
}

// Destination names carry the export name, preset and signal so a list can
// regroup them without extra storage.
const namePrefix = "obs/"

// DestinationName builds the stored name of one signal destination.
func DestinationName(export, preset, signal string) string {
	return namePrefix + export + "/" + preset + "/" + signal
}

// ParseDestinationName splits a stored name made by DestinationName.
func ParseDestinationName(n string) (export, preset, signal string, ok bool) {
	rest, found := strings.CutPrefix(n, namePrefix)
	if !found {
		return "", "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

var exportNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// ValidName reports whether s is an allowed export name.
func ValidName(s string) bool { return exportNameRe.MatchString(s) }

var statusRe = regexp.MustCompile(`status (\d{3})`)

// Hint maps a delivery error to the provider's remedy text. It returns the
// HTTP status found in the error (0 if none) and the hint, which may be empty.
func (p Preset) Hint(err error) (int, string) {
	if err == nil {
		return 0, ""
	}
	m := statusRe.FindStringSubmatch(err.Error())
	if m == nil {
		return 0, ""
	}
	code, _ := strconv.Atoi(m[1])
	if h, ok := p.Hints[m[1]]; ok {
		return code, h
	}
	switch {
	case code == 429:
		return code, "The provider is rate limiting this account. Try again in a minute."
	case code >= 500:
		return code, "The provider had a server error. Try again shortly."
	case code >= 400:
		return code, "The provider rejected the request. Check the fields and the provider's ingest settings."
	}
	return code, ""
}
