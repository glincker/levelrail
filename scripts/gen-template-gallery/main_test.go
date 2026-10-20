package main

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

func TestSanitize(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"em dash spaced", "a \u2014 b", "a, b"},
		{"em dash tight", "a\u2014b", "a, b"},
		{"en dash range", "1\u20133", "1-3"},
		{"en dash spaced", "a \u2013 b", "a, b"},
		{"plain", "hello", "hello"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitize(tc.in); got != tc.want {
				t.Errorf("sanitize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBuildEntryEnvKinds(t *testing.T) {
	tpl := catalog.Template{
		ID: "x", Name: "X", Category: "Test",
		Compose: "services:\n  app:\n    image: x:1\n    ports: [\"8080:80\"]\n    volumes:\n      - data:/data\n    environment:\n      PW: $SERVICE_PASSWORD_DB\n      MODE: prod\n      TOKEN: $SERVICE_CUSTOM_TOKEN\n",
	}
	e, err := buildEntry(tpl)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, v := range e.Env {
		got[v.Key] = v.Kind
	}
	want := map[string]string{"PW": kindGenerated, "MODE": kindPreset, "TOKEN": kindRequired}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("env %s kind = %q, want %q", k, got[k], w)
		}
	}
	if !e.NeedsConfig {
		t.Error("NeedsConfig = false, want true")
	}
	if len(e.Services) != 1 || e.Services[0].Ports[0] != 80 {
		t.Errorf("services = %+v", e.Services)
	}
}

func TestGenerateWholeCatalog(t *testing.T) {
	data, err := generate(catalog.Templates)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(string(data), "\u2014\u2013") {
		t.Error("generated data contains an em or en dash")
	}
}
