package platformimport

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/platformimport/fakevercel"
)

func vercelDiscover(t *testing.T, fake *fakevercel.Server) *Discovery {
	t.Helper()
	srv := httptest.NewServer(fake.Handler())
	t.Cleanup(srv.Close)
	src, err := NewVercel(srv.URL, fakevercel.Token, testOpts)
	if err != nil {
		t.Fatal(err)
	}
	d, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v := fake.Violations(); len(v) != 0 {
		t.Fatalf("source was written to: %v", v)
	}
	return d
}

func TestVercelDiscover(t *testing.T) {
	d := vercelDiscover(t, fakevercel.New())
	// Pagination: two projects on page one, one on page two.
	if len(d.Apps) != 3 || d.Apps[0].Name != "web" || d.Apps[1].Name != "docs" || d.Apps[2].Name != "bare" {
		t.Fatalf("apps: %+v", d.Apps)
	}
	web := d.Apps[0]
	if web.Kind != SourceGit || web.GitURL != "https://github.com/acme/mono" || web.GitBranch != "main" || web.BuildPath != "apps/web" || web.BuildPack != "nextjs" {
		t.Errorf("web: %+v", web)
	}
	if g := d.Apps[1]; g.GitURL != "https://gitlab.com/acme/docs" {
		t.Errorf("docs: %+v", g)
	}
	bare := d.Apps[2]
	if bare.Kind != SourceUnknown || len(bare.Notes) == 0 || !strings.Contains(bare.Notes[0].Manual, "import <repo>") {
		t.Errorf("bare: %+v", bare)
	}
	notes := ""
	for _, n := range web.Notes {
		notes += n.Reason + "\n"
	}
	for _, want := range []string{"pnpm build", "pnpm i", ".next", "20.x"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes miss %q: %s", want, notes)
		}
	}
	all := strings.Join(d.Warnings, "\n")
	for _, want := range []string{"vercel.json", "Postgres maps to", "Edge Config has no equivalent (none)", "project web: env vars differ by target"} {
		if !strings.Contains(all, want) {
			t.Errorf("warnings miss %q:\n%s", want, all)
		}
	}
	if !strings.Contains(all, "PREVIEW_ONLY (preview)") || !strings.Contains(all, "FEATURE (development+production)") {
		t.Errorf("target warning: %s", all)
	}
}

func TestVercelSensitiveEnvKeysOnly(t *testing.T) {
	fake := fakevercel.New()
	d := vercelDiscover(t, fake)
	web := d.Apps[0]
	for _, k := range []string{"API_KEY", "DB_PASS"} {
		if e := envOf(web, k); !e.Secret || e.Value != "" {
			t.Errorf("%s should be a secret without a value: %+v", k, e)
		}
	}
	if e := envOf(web, "NODE_ENV"); e.Secret || e.Value != "production" {
		t.Errorf("NODE_ENV: %+v", e)
	}
	if e := envOf(web, "FEATURE"); e.Value != "prod-on" {
		t.Errorf("FEATURE should use the production value: %+v", e)
	}
	all := strings.Join(d.Warnings, "\n")
	if !strings.Contains(all, "value not readable, set it manually: API_KEY, DB_PASS") || strings.Contains(all, fakevercel.SecretVal) {
		t.Errorf("warnings: %s", all)
	}
	for _, q := range fake.Queries() {
		if strings.Contains(q, "decrypt=true") {
			t.Errorf("must not decrypt: %s", q)
		}
	}
}

func TestVercelDomainsFilterAndPaginate(t *testing.T) {
	d := vercelDiscover(t, fakevercel.New())
	got := strings.Join(d.Apps[0].Domains, ",")
	// vercel.app and redirect-only names are dropped, page two is read.
	if got != "web.example.com,www.example.com" {
		t.Errorf("domains: %s", got)
	}
}

func TestVercelEmptyTeam(t *testing.T) {
	fake := fakevercel.New()
	fake.Empty = true
	d := vercelDiscover(t, fake)
	if len(d.Apps) != 0 || len(d.Databases) != 0 || len(d.Warnings) != 0 {
		t.Fatalf("d=%+v", d)
	}
}

func TestVercelTeamIDSent(t *testing.T) {
	fake := fakevercel.New()
	srv := httptest.NewServer(fake.Handler())
	defer srv.Close()
	src, _ := NewVercel(srv.URL, fakevercel.Token, testOpts)
	if _, err := src.WithTeam(fakevercel.TeamID).Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, q := range fake.Queries() {
		if !strings.Contains(q, "teamId="+fakevercel.TeamID) {
			t.Errorf("query without teamId: %s", q)
		}
	}
}

func TestVercelUnauthorizedRedactsToken(t *testing.T) {
	srv := httptest.NewServer(fakevercel.New().Handler())
	defer srv.Close()
	src, _ := NewVercel(srv.URL, "wrong-token-fixture", testOpts)
	_, err := src.Discover(context.Background())
	var api *APIError
	if err == nil || !errors.As(err, &api) || api.Status != 401 || strings.Contains(err.Error(), "wrong-token-fixture") {
		t.Fatalf("want redacted 401, got %v", err)
	}
}

func TestVercelPlatformWiring(t *testing.T) {
	if p, err := ParsePlatform("vercel"); err != nil || p != Vercel {
		t.Fatalf("ParsePlatform: %v %v", p, err)
	}
	if _, err := NewSource(Vercel, "", "tok", testOpts); err != nil {
		t.Errorf("empty url should use the public API: %v", err)
	}
	if _, err := ParsePlatform("nope"); err == nil || !strings.Contains(err.Error(), "vercel") {
		t.Errorf("error text: %v", err)
	}
}

func TestNewVercelTeamOption(t *testing.T) {
	opts := testOpts
	opts.Team = "team_acme"
	src, err := NewVercel("http://127.0.0.1:1", "tok", opts)
	if err != nil || src.TeamID != "team_acme" {
		t.Fatalf("src=%+v err=%v", src, err)
	}
	opts.Team = "x&y"
	if _, err := NewVercel("http://127.0.0.1:1", "tok", opts); err == nil {
		t.Error("malformed team must be rejected")
	}
	opts.Team = "team_acme"
	if _, err := NewSource(Coolify, "http://127.0.0.1:1", "tok", opts); err == nil {
		t.Error("team on another platform must be rejected")
	}
}
