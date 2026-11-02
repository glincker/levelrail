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

func TestTrustLevels(t *testing.T) {
	signed := &catalog.SignedBy{Method: "keyless", Identity: "x"}
	e := Entry{Images: []Image{{SourceClass: catalog.SourceUpstream, Signed: signed, Provenance: true}}}
	hist := catalog.VerifiedRecord{Arches: []string{"arm64"}}
	got := trustFor(e, hist, true)
	if got.Level != trustVerifiedSigned || got.EvidenceURL != statusPageURL {
		t.Errorf("arm64 local pass = %+v, want signed with status page evidence", got)
	}
	e.Images[0].Signed = nil
	got = trustFor(e, hist, true)
	if got.Level != trustVerified {
		t.Errorf("arm64 local pass unsigned = %q, want verified", got.Level)
	}
	if got := trustFor(e, catalog.VerifiedRecord{}, true).Level; got != trustListed {
		t.Errorf("record without arches = %q, want listed", got)
	}
	e.Images[0].Signed = signed
	ci := catalog.VerifiedRecord{Arches: []string{"amd64"}, RunURL: "https://example.com/run/1"}
	if got := trustFor(e, ci, true).Level; got != trustVerifiedSigned {
		t.Errorf("signed CI run = %q, want verified_signed", got)
	}
	e.Images[0].Signed = nil
	if got := trustFor(e, ci, true).Level; got != trustVerified {
		t.Errorf("unsigned CI run = %q, want verified", got)
	}
	if got := trustFor(e, ci, false).Level; got != trustListed {
		t.Errorf("no record = %q, want listed", got)
	}
}

func TestArchStates(t *testing.T) {
	imgs := []Image{{Platforms: []string{"linux/amd64"}}}
	got := archStates(imgs, catalog.VerifiedRecord{Arches: []string{"amd64"}}, true)
	if got["amd64"] != "tested" || got["arm64"] != "absent" {
		t.Errorf("archStates = %v", got)
	}
	if archStates([]Image{{}}, catalog.VerifiedRecord{}, false)["amd64"] != "unknown" {
		t.Error("unscanned image must be unknown")
	}
}
