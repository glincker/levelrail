package apptype

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Schema is the constrained JSON Schema subset a type uses for parameters.
// Composition, references and remote ids are left out so a form and a
// one-line error per field stay predictable.
type Schema struct {
	Type        string
	Enum        []any
	Minimum     *float64
	Maximum     *float64
	MinLength   *int
	MaxLength   *int
	Pattern     string
	Format      string
	Default     any
	Description string
	Required    []string
	Properties  map[string]*Schema

	re *regexp.Regexp
}

var allowedKeywords = map[string]bool{
	"type": true, "enum": true, "minimum": true, "maximum": true, "minLength": true,
	"maxLength": true, "pattern": true, "default": true, "description": true,
	"required": true, "properties": true, "additionalProperties": true, "format": true,
}

func allowedKeywordList() string {
	keys := make([]string, 0, len(allowedKeywords))
	for k := range allowedKeywords {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// ParseSchema checks raw against the subset and the env-driven limits.
func ParseSchema(raw json.RawMessage, lim Limits) (*Schema, error) {
	if len(raw) == 0 {
		return &Schema{Type: "object", Properties: map[string]*Schema{}}, nil
	}
	if len(raw) > lim.MaxSchemaBytes {
		return nil, fmt.Errorf("parameters: schema is %d bytes, the limit is %d", len(raw), lim.MaxSchemaBytes)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parameters: not a JSON object: %w", err)
	}
	count := 0
	s, err := buildSchema(m, "parameters", 0, &count, lim)
	if err != nil {
		return nil, err
	}
	if s.Type != "object" {
		return nil, fmt.Errorf("parameters: the root must have type object")
	}
	return s, nil
}

func buildSchema(m map[string]any, path string, depth int, count *int, lim Limits) (*Schema, error) {
	for k := range m {
		if !allowedKeywords[k] {
			return nil, fmt.Errorf("%s: keyword %q is not supported (allowed: %s)", path, k, allowedKeywordList())
		}
	}
	s := &Schema{}
	t, _ := m["type"].(string)
	switch t {
	case "string", "integer", "boolean", "object":
		s.Type = t
	default:
		return nil, fmt.Errorf("%s: type must be string, integer, boolean or object", path)
	}
	if err := checkApplicable(m, t, path); err != nil {
		return nil, err
	}
	s.Description, _ = m["description"].(string)
	if err := readBounds(s, m, path); err != nil {
		return nil, err
	}
	if p, ok := m["pattern"].(string); ok {
		if len(p) > lim.MaxPatternLen {
			return nil, fmt.Errorf("%s: pattern is %d characters, the limit is %d", path, len(p), lim.MaxPatternLen)
		}
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("%s: pattern is not valid RE2: %w", path, err)
		}
		s.Pattern, s.re = p, re
	}
	if f, ok := m["format"].(string); ok {
		if f != "cron" {
			return nil, fmt.Errorf("%s: format %q is not supported, only cron", path, f)
		}
		s.Format = f
	}
	if e, ok := m["enum"]; ok {
		list, isList := e.([]any)
		if !isList || len(list) == 0 {
			return nil, fmt.Errorf("%s: enum must be a non-empty list", path)
		}
		s.Enum = list
	}
	if t == "object" {
		if err := buildProperties(s, m, path, depth, count, lim); err != nil {
			return nil, err
		}
	}
	if d, ok := m["default"]; ok {
		s.Default = d
		if t != "object" {
			if msgs := s.Validate(path+".default", d); len(msgs) > 0 {
				return nil, fmt.Errorf("%s", msgs[0])
			}
		}
	}
	return s, nil
}

func checkApplicable(m map[string]any, t, path string) error {
	only := map[string]string{
		"minimum": "integer", "maximum": "integer",
		"minLength": "string", "maxLength": "string", "pattern": "string", "format": "string",
		"required": "object", "properties": "object", "additionalProperties": "object",
	}
	for k, want := range only {
		if _, ok := m[k]; ok && t != want {
			return fmt.Errorf("%s: keyword %q only applies to type %s", path, k, want)
		}
	}
	if _, ok := m["enum"]; ok && (t == "object" || t == "boolean") {
		return fmt.Errorf("%s: enum only applies to string or integer", path)
	}
	return nil
}

func readBounds(s *Schema, m map[string]any, path string) error {
	for _, k := range []string{"minimum", "maximum"} {
		if v, ok := m[k]; ok {
			f, isNum := v.(float64)
			if !isNum {
				return fmt.Errorf("%s: %s must be a number", path, k)
			}
			if k == "minimum" {
				s.Minimum = &f
			} else {
				s.Maximum = &f
			}
		}
	}
	for _, k := range []string{"minLength", "maxLength"} {
		if v, ok := m[k]; ok {
			f, isNum := v.(float64)
			if !isNum || f < 0 || f != math.Trunc(f) {
				return fmt.Errorf("%s: %s must be a non-negative integer", path, k)
			}
			n := int(f)
			if k == "minLength" {
				s.MinLength = &n
			} else {
				s.MaxLength = &n
			}
		}
	}
	if s.Minimum != nil && s.Maximum != nil && *s.Minimum > *s.Maximum {
		return fmt.Errorf("%s: minimum is above maximum", path)
	}
	return nil
}

func buildProperties(s *Schema, m map[string]any, path string, depth int, count *int, lim Limits) error {
	if ap, ok := m["additionalProperties"]; ok && ap != false {
		return fmt.Errorf("%s: additionalProperties must be false, it is always forced", path)
	}
	props, _ := m["properties"].(map[string]any)
	s.Properties = map[string]*Schema{}
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sub, ok := props[name].(map[string]any)
		if !ok {
			return fmt.Errorf("%s.%s: must be an object", path, name)
		}
		if depth >= 1 && sub["type"] == "object" {
			return fmt.Errorf("%s.%s: objects can nest one level only", path, name)
		}
		child, err := buildSchema(sub, path+"."+name, depth+1, count, lim)
		if err != nil {
			return err
		}
		if child.Type != "object" {
			*count++
			if *count > lim.MaxParams {
				return fmt.Errorf("%s: more than %d parameters, the limit is %d", path, lim.MaxParams, lim.MaxParams)
			}
		}
		s.Properties[name] = child
	}
	if req, ok := m["required"]; ok {
		list, isList := req.([]any)
		if !isList {
			return fmt.Errorf("%s: required must be a list of names", path)
		}
		for _, r := range list {
			name, _ := r.(string)
			if _, known := s.Properties[name]; !known {
				return fmt.Errorf("%s: required names %q which is not a property", path, r)
			}
			s.Required = append(s.Required, name)
		}
	}
	return nil
}

// ParamPaths lists the dotted path of every scalar parameter, sorted.
func (s *Schema) ParamPaths() []string {
	var out []string
	var walk func(prefix string, n *Schema)
	walk = func(prefix string, n *Schema) {
		for name, child := range n.Properties {
			p := name
			if prefix != "" {
				p = prefix + "." + name
			}
			if child.Type == "object" {
				walk(p, child)
			} else {
				out = append(out, p)
			}
		}
	}
	walk("", s)
	sort.Strings(out)
	return out
}

// Validate returns one readable message per problem with v at path.
func (s *Schema) Validate(path string, v any) []string {
	switch s.Type {
	case "object":
		return s.validateObject(path, v)
	case "boolean":
		if _, ok := v.(bool); !ok {
			return []string{fmt.Sprintf("%s must be true or false", path)}
		}
	case "integer":
		return s.validateInteger(path, v)
	case "string":
		return s.validateString(path, v)
	}
	return nil
}

func (s *Schema) validateObject(path string, v any) []string {
	obj, ok := v.(map[string]any)
	if !ok {
		return []string{fmt.Sprintf("%s must be an object", path)}
	}
	var msgs []string
	names := make([]string, 0, len(obj))
	for k := range obj {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		child, known := s.Properties[k]
		if !known {
			msgs = append(msgs, fmt.Sprintf("%s.%s is not a parameter of this type", path, k))
			continue
		}
		msgs = append(msgs, child.Validate(path+"."+k, obj[k])...)
	}
	for _, r := range s.Required {
		if _, has := obj[r]; !has && s.Properties[r].Default == nil {
			msgs = append(msgs, fmt.Sprintf("%s.%s is required", path, r))
		}
	}
	return msgs
}

func (s *Schema) validateInteger(path string, v any) []string {
	f, ok := toFloat(v)
	if !ok || f != math.Trunc(f) {
		return []string{fmt.Sprintf("%s must be a whole number", path)}
	}
	var msgs []string
	if s.Minimum != nil && f < *s.Minimum {
		msgs = append(msgs, fmt.Sprintf("%s is %v, the minimum is %v", path, f, *s.Minimum))
	}
	if s.Maximum != nil && f > *s.Maximum {
		msgs = append(msgs, fmt.Sprintf("%s is %v, the maximum is %v", path, f, *s.Maximum))
	}
	if len(s.Enum) > 0 && !inEnum(s.Enum, f) {
		msgs = append(msgs, fmt.Sprintf("%s must be one of %s", path, enumList(s.Enum)))
	}
	return msgs
}

func (s *Schema) validateString(path string, v any) []string {
	str, ok := v.(string)
	if !ok {
		return []string{fmt.Sprintf("%s must be text", path)}
	}
	var msgs []string
	if s.MinLength != nil && len(str) < *s.MinLength {
		msgs = append(msgs, fmt.Sprintf("%s is shorter than %d characters", path, *s.MinLength))
	}
	if s.MaxLength != nil && len(str) > *s.MaxLength {
		msgs = append(msgs, fmt.Sprintf("%s is longer than %d characters", path, *s.MaxLength))
	}
	if s.re != nil && !s.re.MatchString(str) {
		msgs = append(msgs, fmt.Sprintf("%s does not match the pattern %s", path, s.Pattern))
	}
	if s.Format == "cron" && len(strings.Fields(str)) != 5 {
		msgs = append(msgs, fmt.Sprintf("%s must be a 5 field cron expression", path))
	}
	if len(s.Enum) > 0 && !inEnum(s.Enum, str) {
		msgs = append(msgs, fmt.Sprintf("%s must be one of %s", path, enumList(s.Enum)))
	}
	return msgs
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}

func inEnum(enum []any, v any) bool {
	for _, e := range enum {
		if e == v {
			return true
		}
	}
	return false
}

func enumList(enum []any) string {
	parts := make([]string, len(enum))
	for i, e := range enum {
		parts[i] = fmt.Sprint(e)
	}
	return strings.Join(parts, ", ")
}
