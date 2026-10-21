package forms

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Field input types a form can declare.
const (
	TypeText     = "text"
	TypeEmail    = "email"
	TypeTextarea = "textarea"
	TypeSelect   = "select"
	TypeCheckbox = "checkbox"
	TypeNumber   = "number"
	TypeURL      = "url"
	TypeHidden   = "hidden"
)

const (
	maxSchemaFields  = 50
	maxSelectOptions = 100
	maxLabelLen      = 200
	maxSuccessLen    = 500
	maxOrigins       = 20
)

var fieldNameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

// Field is one input of a form.
type Field struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Label       string   `json:"label,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	MinLength   int      `json:"min_length,omitempty"`
	MaxLength   int      `json:"max_length,omitempty"`
	Options     []string `json:"options,omitempty"`
	// Default is the value a hidden field carries in the snippet.
	Default string `json:"default,omitempty"`
}

// Schema is a form's editable design. No fields means the default three (DefaultFields).
// With fields set, submitted names that are not declared are dropped, not stored.
type Schema struct {
	Fields         []Field  `json:"fields"`
	SuccessMessage string   `json:"success_message,omitempty"`
	AllowedOrigins []string `json:"allowed_origins,omitempty"`
}

// DefaultFields is the layout used by forms that never set a schema.
func DefaultFields() []Field {
	return []Field{
		{Name: "name", Type: TypeText, Label: "Name", Required: true},
		{Name: "email", Type: TypeEmail, Label: "Email", Required: true},
		{Name: "message", Type: TypeTextarea, Label: "Message"},
	}
}

// EffectiveFields returns the declared fields, or the default three when none are set.
func (s Schema) EffectiveFields() []Field {
	if len(s.Fields) == 0 {
		return DefaultFields()
	}
	return s.Fields
}

// ParseSchema decodes the stored JSON; empty or "{}" yields the zero Schema.
func ParseSchema(raw string) (Schema, error) {
	var s Schema
	if strings.TrimSpace(raw) == "" {
		return s, nil
	}
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return Schema{}, fmt.Errorf("forms: parse schema: %w", err)
	}
	return s, nil
}

// ValidateSchema checks the design itself, not a submission.
func ValidateSchema(s Schema, honeypot string) error {
	if len(s.Fields) > maxSchemaFields {
		return errors.New("a form can have at most 50 fields")
	}
	if utf8.RuneCountInString(s.SuccessMessage) > maxSuccessLen {
		return errors.New("success_message is too long")
	}
	if len(s.AllowedOrigins) > maxOrigins {
		return errors.New("too many allowed origins")
	}
	for _, o := range s.AllowedOrigins {
		u, err := url.Parse(o)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" || u.User != nil {
			return fmt.Errorf("allowed origin %q must look like https://example.com", o)
		}
	}
	seen := map[string]bool{}
	for _, f := range s.Fields {
		if err := validateFieldDef(f); err != nil {
			return err
		}
		if seen[f.Name] {
			return fmt.Errorf("field name %q is used twice", f.Name)
		}
		if honeypot != "" && f.Name == honeypot {
			return fmt.Errorf("field name %q is the spam trap field", f.Name)
		}
		seen[f.Name] = true
	}
	return nil
}

func validateFieldDef(f Field) error {
	if !fieldNameRE.MatchString(f.Name) {
		return fmt.Errorf("field name %q must start with a letter and use letters, digits, dashes or underscores", f.Name)
	}
	switch f.Type {
	case TypeText, TypeEmail, TypeTextarea, TypeCheckbox, TypeNumber, TypeURL, TypeHidden:
	case TypeSelect:
		if len(f.Options) == 0 || len(f.Options) > maxSelectOptions {
			return fmt.Errorf("field %q needs between 1 and 100 options", f.Name)
		}
	default:
		return fmt.Errorf("field %q has unknown type %q", f.Name, f.Type)
	}
	if f.MinLength < 0 || f.MaxLength < 0 || (f.MaxLength > 0 && f.MinLength > f.MaxLength) {
		return fmt.Errorf("field %q has an invalid length range", f.Name)
	}
	if utf8.RuneCountInString(f.Label) > maxLabelLen || utf8.RuneCountInString(f.Placeholder) > maxLabelLen {
		return fmt.Errorf("field %q label or placeholder is too long", f.Name)
	}
	return nil
}

// ValidateSubmission checks fields against the schema. It returns the cleaned map (declared
// names plus the honeypot key, unknown names dropped) and a message per invalid field.
// A schema with no fields accepts everything unchanged.
func ValidateSubmission(s Schema, honeypot string, in map[string]string) (map[string]string, map[string]string) {
	if len(s.Fields) == 0 {
		return in, nil
	}
	out := make(map[string]string, len(s.Fields)+1)
	problems := map[string]string{}
	if honeypot != "" {
		if v, ok := in[honeypot]; ok {
			out[honeypot] = v
		}
	}
	for _, f := range s.Fields {
		v := strings.TrimSpace(in[f.Name])
		if v == "" {
			if f.Required {
				problems[f.Name] = "required"
			}
			continue
		}
		if msg := checkValue(f, v); msg != "" {
			problems[f.Name] = msg
			continue
		}
		out[f.Name] = v
	}
	if len(problems) == 0 {
		problems = nil
	}
	return out, problems
}

func checkValue(f Field, v string) string {
	n := utf8.RuneCountInString(v)
	if f.MinLength > 0 && n < f.MinLength {
		return "too_short"
	}
	if f.MaxLength > 0 && n > f.MaxLength {
		return "too_long"
	}
	switch f.Type {
	case TypeEmail:
		if a, err := mail.ParseAddress(v); err != nil || a.Address != v {
			return "invalid_email"
		}
	case TypeURL:
		if u, err := url.Parse(v); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "invalid_url"
		}
	case TypeNumber:
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			return "invalid_number"
		}
	case TypeSelect:
		for _, o := range f.Options {
			if o == v {
				return ""
			}
		}
		return "invalid_option"
	}
	return ""
}

// SampleValues builds a plausible submission for the test button.
func SampleValues(s Schema) map[string]string {
	out := map[string]string{}
	for _, f := range s.EffectiveFields() {
		v := "Test submission"
		switch f.Type {
		case TypeEmail:
			v = "test@example.com"
		case TypeNumber:
			v = "1"
		case TypeURL:
			v = "https://example.com"
		case TypeSelect:
			v = f.Options[0]
		case TypeCheckbox:
			v = "on"
		case TypeHidden:
			v = f.Default
		}
		if v != "" {
			out[f.Name] = v
		}
	}
	return out
}
