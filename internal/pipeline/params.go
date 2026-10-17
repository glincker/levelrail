package pipeline

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Parameter types.
const (
	ParamString  = "string"
	ParamChoice  = "choice"
	ParamBoolean = "boolean"
)

var paramNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// Parameter is one declared run parameter, set by a manual run, an API
// call, or a webhook query string.
type Parameter struct {
	Name        string   `yaml:"name" json:"name"`
	Type        string   `yaml:"type,omitempty" json:"type,omitempty"`
	Default     string   `yaml:"default,omitempty" json:"default,omitempty"`
	Choices     []string `yaml:"choices,omitempty" json:"choices,omitempty"`
	Required    bool     `yaml:"required,omitempty" json:"required,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
}

// EffectiveType returns the declared type, inferred when left blank.
func (p Parameter) EffectiveType() string {
	switch {
	case p.Type != "":
		return p.Type
	case len(p.Choices) > 0:
		return ParamChoice
	}
	return ParamString
}

// DeclaredParameters lists every parameter the pipeline declares: the top-level
// `parameters` list plus the older `on.manual.inputs` map, by name.
func (d *Definition) DeclaredParameters() []Parameter {
	out := slices.Clone(d.Parameters)
	if d.On.Manual != nil {
		names := make([]string, 0, len(d.On.Manual.Inputs))
		for n := range d.On.Manual.Inputs {
			names = append(names, n)
		}
		slices.Sort(names)
		for _, n := range names {
			in := d.On.Manual.Inputs[n]
			out = append(out, Parameter{Name: n, Type: in.Type, Default: in.Default, Choices: in.ChoiceList(), Required: in.Required, Description: in.Description})
		}
	}
	return out
}

// ChoiceList returns the allowed values, from choices or the older options.
func (in Input) ChoiceList() []string {
	if len(in.Choices) > 0 {
		return in.Choices
	}
	return in.Options
}

func validateParameter(p Parameter) string {
	if !paramNameRe.MatchString(p.Name) {
		return fmt.Sprintf("parameter name %q must start with a letter and use letters, digits, or underscores", p.Name)
	}
	typ := p.EffectiveType()
	switch typ {
	case ParamString, ParamChoice, ParamBoolean:
	default:
		return fmt.Sprintf("unknown parameter type %q (use string, choice, or boolean)", typ)
	}
	if typ == ParamChoice && len(p.Choices) == 0 {
		return "a choice parameter needs choices"
	}
	if typ != ParamChoice && len(p.Choices) > 0 {
		return "choices only apply to a choice parameter"
	}
	if p.Default != "" {
		if _, err := checkParamValue(p, p.Default); err != nil {
			return "default: " + err.Error()
		}
	}
	return ""
}

func checkParamValue(p Parameter, v string) (string, error) {
	switch p.EffectiveType() {
	case ParamBoolean:
		switch strings.ToLower(v) {
		case "true":
			return "true", nil
		case "false":
			return "false", nil
		}
		return "", fmt.Errorf("must be true or false")
	case ParamChoice:
		if !slices.Contains(p.Choices, v) {
			return "", fmt.Errorf("must be one of %s", strings.Join(p.Choices, ", "))
		}
	}
	return v, nil
}

// ResolveParameters validates the supplied values against the declared
// parameters and returns the values the run records, defaults filled in.
func ResolveParameters(def *Definition, given map[string]string, maxValueBytes int) (map[string]string, error) {
	declared := def.DeclaredParameters()
	byName := make(map[string]Parameter, len(declared))
	for _, p := range declared {
		byName[p.Name] = p
	}
	for k := range given {
		if _, ok := byName[k]; !ok {
			return nil, fmt.Errorf("%w: unknown input %q", ErrInvalidInput, k)
		}
	}
	out := make(map[string]string, len(declared))
	for _, p := range declared {
		v, ok := given[p.Name]
		if !ok {
			v = p.Default
		}
		if maxValueBytes > 0 && len(v) > maxValueBytes {
			return nil, fmt.Errorf("%w: input %q is longer than %d bytes", ErrInvalidInput, p.Name, maxValueBytes)
		}
		if v == "" && p.EffectiveType() == ParamBoolean && !p.Required {
			v = "false"
		}
		if v == "" {
			if p.Required {
				return nil, fmt.Errorf("%w: input %q is required", ErrInvalidInput, p.Name)
			}
			out[p.Name] = ""
			continue
		}
		norm, err := checkParamValue(p, v)
		if err != nil {
			return nil, fmt.Errorf("%w: input %q %s", ErrInvalidInput, p.Name, err)
		}
		out[p.Name] = norm
	}
	return out, nil
}
