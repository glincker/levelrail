package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/catalog"
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
	reg, err := catalog.LoadRegistryEntries("../../registry/templates")
	if err != nil {
		t.Fatal(err)
	}
	data, err := generate(catalog.Templates, reg)
	if err != nil {
		t.Fatal(err)
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatal(err)
	}
	if want := len(catalog.Templates) + len(registryOnly(catalog.Templates, reg)); len(idx.Templates) != want {
		t.Errorf("index has %d templates, want union %d", len(idx.Templates), want)
	}
	if strings.ContainsAny(string(data), "\u2014\u2013") {
		t.Error("generated data contains an em or en dash")
	}
}

func TestInstallFor(t *testing.T) {
	if in := installFor(nil); in.Kind != "deploy" || len(in.Fields) != 0 {
		t.Errorf("nil manifest = %+v", in)
	}
	m := &catalog.Manifest{Env: []catalog.EnvPrompt{
		{Key: "A", Label: "Admin", Required: true},
		{Key: "B", Default: "x", Required: true},
		{Key: "C", Generate: "password"},
	}}
	in := installFor(m)
	if in.Kind != "install" || len(in.Fields) != 3 {
		t.Fatalf("install = %+v", in)
	}
	want := []InstallField{
		{"A", "Admin", true, "none"},
		{"B", "B", false, "preset"},
		{"C", "C", false, "generated"},
	}
	for i, w := range want {
		if in.Fields[i] != w {
			t.Errorf("field %d = %+v, want %+v", i, in.Fields[i], w)
		}
	}
}
