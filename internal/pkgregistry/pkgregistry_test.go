package pkgregistry

import (
	"strings"
	"testing"
)

func TestRender_PerEcosystem(t *testing.T) {
	cases := []struct {
		name     string
		entry    Entry
		secretID string
		want     []string
		wantArgs map[string]string
	}{
		{"npm token", Entry{Ecosystem: EcosystemNPM, URL: "https://npm.example.com/repo", Secret: "tok-123"}, "npmrc",
			[]string{"registry=https://npm.example.com/repo/", "//npm.example.com/repo/:_authToken=tok-123"}, nil},
		{"pypi", Entry{Ecosystem: EcosystemPyPI, URL: "https://pypi.example.com/simple", Username: "bob", Secret: "p@ss"}, "pip_conf",
			[]string{"index-url = https://bob:p%40ss@pypi.example.com/simple"}, nil},
		{"maven", Entry{Ecosystem: EcosystemMaven, URL: "https://mvn.example.com/r", Username: "u", Secret: "a&b"}, "maven_settings",
			[]string{"<password>a&amp;b</password>", "<url>https://mvn.example.com/r</url>"}, nil},
		{"nuget", Entry{Ecosystem: EcosystemNuGet, URL: "https://nuget.example.com/v3/index.json", Username: "u", Secret: "s"}, "nuget_config",
			[]string{`key="ClearTextPassword" value="s"`}, nil},
		{"cargo", Entry{Ecosystem: EcosystemCargo, URL: "https://cargo.example.com/index", Secret: "ct"}, "cargo_config",
			[]string{`index = "sparse+https://cargo.example.com/index/"`, `token = "ct"`}, nil},
		{"go", Entry{Ecosystem: EcosystemGo, URL: "https://goproxy.example.com", Username: "u", Secret: "gs", ModulePatterns: "example.com/*"}, "netrc",
			[]string{"machine goproxy.example.com login u password gs"},
			map[string]string{"GOPROXY": "https://goproxy.example.com,direct", "GOPRIVATE": "example.com/*"}},
		{"rubygems", Entry{Ecosystem: EcosystemRubyGems, URL: "https://gems.example-host.com", Username: "u", Secret: "gs"}, "bundle_config",
			[]string{`BUNDLE_GEMS__EXAMPLE___HOST__COM: "u:gs"`}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Render([]Entry{tc.entry})
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			body := string(out.Files[tc.secretID])
			for _, w := range tc.want {
				if !strings.Contains(body, w) {
					t.Errorf("%s missing %q in:\n%s", tc.secretID, w, body)
				}
			}
			for k, v := range tc.wantArgs {
				if out.Args[k] != v {
					t.Errorf("arg %s = %q, want %q", k, out.Args[k], v)
				}
			}
			for _, v := range out.Args {
				if strings.Contains(v, tc.entry.Secret) {
					t.Errorf("a build arg carries the secret: %q", v)
				}
			}
		})
	}
}

func TestRender_RejectsEmbeddedCredentialsAndFirstWins(t *testing.T) {
	if _, err := Render([]Entry{{Ecosystem: EcosystemNPM, URL: "https://u:p@npm.example.com"}}); err == nil { //nolint:gosec // fake userinfo, the case under test
		t.Fatal("expected an error for a URL with credentials")
	}
	out, err := Render([]Entry{
		{Ecosystem: EcosystemNPM, URL: "https://app.example.com", Secret: "app-token"},
		{Ecosystem: EcosystemNPM, URL: "https://inst.example.com", Secret: "inst-token"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out.Files["npmrc"]), "app-token") || strings.Contains(string(out.Files["npmrc"]), "inst-token") {
		t.Errorf("first entry should win: %s", out.Files["npmrc"])
	}
}

func TestRedactor_MasksSecretForms(t *testing.T) {
	out, _ := Render([]Entry{{Ecosystem: EcosystemPyPI, URL: "https://p.example.com/simple", Username: "u", Secret: "s3cret/val"}})
	mask := Redactor(out.Redact)
	got := mask("pip --index-url https://u:s3cret%2Fval@p.example.com and s3cret/val")
	if strings.Contains(got, "s3cret") {
		t.Errorf("redaction leaked: %s", got)
	}
}
