package forms

import (
	"strings"
	"testing"
)

func TestValidateSubmission(t *testing.T) {
	schema := Schema{Fields: []Field{
		{Name: "email", Type: TypeEmail, Required: true},
		{Name: "msg", Type: TypeTextarea, MinLength: 3, MaxLength: 10},
		{Name: "plan", Type: TypeSelect, Options: []string{"free", "pro"}},
		{Name: "qty", Type: TypeNumber},
		{Name: "site", Type: TypeURL},
		{Name: "agree", Type: TypeCheckbox, Required: true},
	}}
	tests := []struct {
		name     string
		schema   Schema
		in       map[string]string
		wantBad  map[string]string
		wantKeys []string
	}{
		{"valid, unknown dropped", schema, map[string]string{"email": "a@b.co", "agree": "on", "junk": "x", "bot-field": ""}, nil, []string{"agree", "bot-field", "email"}},
		{"missing required", schema, map[string]string{"agree": "on"}, map[string]string{"email": "required"}, nil},
		{"bad email", schema, map[string]string{"email": "nope", "agree": "on"}, map[string]string{"email": "invalid_email"}, nil},
		{"too short", schema, map[string]string{"email": "a@b.co", "agree": "on", "msg": "hi"}, map[string]string{"msg": "too_short"}, nil},
		{"too long", schema, map[string]string{"email": "a@b.co", "agree": "on", "msg": strings.Repeat("x", 11)}, map[string]string{"msg": "too_long"}, nil},
		{"bad option", schema, map[string]string{"email": "a@b.co", "agree": "on", "plan": "gold"}, map[string]string{"plan": "invalid_option"}, nil},
		{"bad number and url", schema, map[string]string{"email": "a@b.co", "agree": "on", "qty": "x", "site": "ftp://a"}, map[string]string{"qty": "invalid_number", "site": "invalid_url"}, nil},
		{"required checkbox unchecked", schema, map[string]string{"email": "a@b.co"}, map[string]string{"agree": "required"}, nil},
		{"no fields accepts all", Schema{}, map[string]string{"anything": "goes"}, nil, []string{"anything"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, bad := ValidateSubmission(tt.schema, "bot-field", tt.in)
			if len(bad) != len(tt.wantBad) {
				t.Fatalf("problems = %v, want %v", bad, tt.wantBad)
			}
			for k, v := range tt.wantBad {
				if bad[k] != v {
					t.Fatalf("problems = %v, want %v", bad, tt.wantBad)
				}
			}
			if tt.wantKeys != nil {
				keys := SortedKeys(got)
				if strings.Join(keys, ",") != strings.Join(tt.wantKeys, ",") {
					t.Fatalf("keys = %v, want %v", keys, tt.wantKeys)
				}
			}
		})
	}
}

func TestValidateSchema(t *testing.T) {
	tests := []struct {
		name    string
		schema  Schema
		wantErr bool
	}{
		{"empty ok", Schema{}, false},
		{"ok", Schema{Fields: []Field{{Name: "a", Type: TypeText}}, AllowedOrigins: []string{"https://x.test"}}, false},
		{"duplicate", Schema{Fields: []Field{{Name: "a", Type: TypeText}, {Name: "a", Type: TypeText}}}, true},
		{"bad name", Schema{Fields: []Field{{Name: "1a", Type: TypeText}}}, true},
		{"bad type", Schema{Fields: []Field{{Name: "a", Type: "date"}}}, true},
		{"select without options", Schema{Fields: []Field{{Name: "a", Type: TypeSelect}}}, true},
		{"honeypot clash", Schema{Fields: []Field{{Name: "bot-field", Type: TypeText}}}, true},
		{"bad range", Schema{Fields: []Field{{Name: "a", Type: TypeText, MinLength: 5, MaxLength: 2}}}, true},
		{"origin with path", Schema{AllowedOrigins: []string{"https://x.test/a"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateSchema(tt.schema, "bot-field"); (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
