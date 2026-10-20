package dbupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbcatalogbuild"
)

var fixedNow = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

type fixtureTags struct{ calls *int }

func (f fixtureTags) ListTags(_ context.Context, repo string) ([]string, error) {
	*f.calls++
	raw, err := os.ReadFile(filepath.Join("..", "dbcatalogbuild", "testdata", map[string]string{
		"library/postgres": "postgres.tags.json", "library/redis": "redis.tags.json",
	}[repo]))
	if err != nil {
		return nil, err
	}
	var d struct{ Tags []string }
	return d.Tags, json.Unmarshal(raw, &d)
}

type fixtureCycles struct{}

func (fixtureCycles) Cycles(_ context.Context, slug string) ([]dbcatalogbuild.Cycle, error) {
	raw, err := os.ReadFile(filepath.Join("..", "dbcatalogbuild", "testdata", slug+".eol.json")) //nolint:gosec // fixture name from the test
	if err != nil {
		return nil, err
	}
	var out []dbcatalogbuild.Cycle
	return out, json.Unmarshal(raw, &out)
}

type rateLimitedTags struct{}

func (rateLimitedTags) ListTags(context.Context, string) ([]string, error) {
	return nil, &dbcatalogbuild.RateLimitedError{Source: "registry", RetryAfter: 45 * time.Second}
}

func newRemote(t *testing.T, cfg RemoteConfig) *RemoteCatalog {
	t.Helper()
	cat, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Catalog = cat
	if cfg.DataDir == "" {
		cfg.DataDir = t.TempDir()
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return fixedNow }
	}
	if cfg.Env == nil {
		cfg.Env = func(string) (string, bool) { return "", false }
	}
	return NewRemoteCatalog(cfg)
}

func publishedServer(t *testing.T, body []byte, sig string, hits *int) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/databases.v1.json":
			*hits++
			if r.Header.Get("If-None-Match") == `"v1"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"v1"`)
			_, _ = w.Write(body) //nolint:gosec // test fixture
		case "/databases.v1.json.sig":
			_, _ = w.Write([]byte(sig))
		default:
			http.NotFound(w, r)
		}
	}))
}

func sampleFile(t *testing.T) []byte {
	t.Helper()
	f := dbcatalogbuild.File{Schema: 1, GeneratedAt: fixedNow.Add(-48 * time.Hour).Format(time.RFC3339), Engines: map[string]dbcatalogbuild.Engine{
		"postgres": {
			Versions: []dbcatalogbuild.Version{
				{Version: "17.9", Tag: "17.9", Channel: "stable", Recommended: false},
				{Version: "18.3", Tag: "18.3", Channel: "stable", Recommended: true},
				{Version: "19rc1", Tag: "19rc1", Channel: "rc"},
				{Version: "not a version", Tag: "x", Channel: "stable"},
			},
			Lines: []dbcatalogbuild.Line{{Line: "14", EOL: "2026-01-01"}, {Line: "18", EOL: "2030-11-14"}},
		},
	}}
	raw, err := f.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestPublishedMergeEtagAndGuards(t *testing.T) {
	priv, pub, err := dbcatalogbuild.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	body := sampleFile(t)
	sig, _ := dbcatalogbuild.Sign(priv, body)
	hits := 0
	srv := publishedServer(t, body, sig, &hits)
	defer srv.Close()
	dir := t.TempDir()
	r := newRemote(t, RemoteConfig{DataDir: dir, URL: srv.URL + "/databases.v1.json", PublicKey: pub, HTTP: srv.Client()})
	before, _ := r.cfg.Catalog.Engine("postgres")
	if err := r.RefreshPublished(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ := r.cfg.Catalog.Engine("postgres")
	if after.AutoMax != before.AutoMax || after.ImageRevert != before.ImageRevert || len(after.Advisories) != len(before.Advisories) {
		t.Fatal("remote data changed curated fields")
	}
	has := map[string]bool{}
	for _, v := range after.Versions {
		has[v] = true
	}
	if !has["18.3"] || has["19rc1"] || has["not a version"] {
		t.Fatalf("versions = %v", after.Versions)
	}
	view, err := r.Versions("postgres")
	if err != nil || view.Origin != CatalogSourcePublished || view.Freshness != FreshnessFresh {
		t.Fatalf("view = %+v err = %v", view, err)
	}
	var sawRC, rec bool
	for _, v := range view.Versions {
		sawRC = sawRC || (v.Version == "19rc1" && v.Channel == "rc")
		rec = rec || (v.Version == "18.3" && v.Recommended)
	}
	if !sawRC || !rec {
		t.Fatalf("view versions = %+v", view.Versions)
	}
	if err := r.RefreshPublished(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.Status().LastChecked == "" || hits != 2 {
		t.Fatalf("hits = %d", hits)
	}
	reloaded := newRemote(t, RemoteConfig{DataDir: dir, URL: srv.URL + "/databases.v1.json", PublicKey: pub, HTTP: srv.Client()})
	if v, _ := reloaded.Versions("postgres"); v.Origin != CatalogSourcePublished {
		t.Fatalf("cache not reloaded: %+v", v)
	}
}

func TestPublishedRejectsBadSignatureAndMissingKey(t *testing.T) {
	_, pub, _ := dbcatalogbuild.GenerateKey()
	otherPriv, _, _ := dbcatalogbuild.GenerateKey()
	body := sampleFile(t)
	badSig, _ := dbcatalogbuild.Sign(otherPriv, body)
	hits := 0
	srv := publishedServer(t, body, badSig, &hits)
	defer srv.Close()
	r := newRemote(t, RemoteConfig{URL: srv.URL + "/databases.v1.json", PublicKey: pub, HTTP: srv.Client()})
	if err := r.RefreshPublished(context.Background()); err == nil {
		t.Fatal("bad signature accepted")
	}
	if v, _ := r.Versions("postgres"); v.Origin != CatalogSourceEmbedded {
		t.Fatalf("origin = %s", v.Origin)
	}
	nokey := newRemote(t, RemoteConfig{URL: srv.URL + "/databases.v1.json", HTTP: srv.Client()})
	if err := nokey.RefreshPublished(context.Background()); !errors.Is(err, ErrRemoteDisabled) {
		t.Fatalf("err = %v", err)
	}
	if s := nokey.Status(); s.RemoteEnabled || s.DisabledReason == "" {
		t.Fatalf("status = %+v", s)
	}
}

func TestEnvOffDisablesEverything(t *testing.T) {
	r := newRemote(t, RemoteConfig{PublicKey: "x", Env: func(k string) (string, bool) {
		return "off", k == EnvCatalogRemote
	}})
	if err := r.RefreshPublished(context.Background()); !errors.Is(err, ErrRemoteDisabled) {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := r.LiveRefresh(context.Background(), "postgres"); !errors.Is(err, ErrRemoteDisabled) {
		t.Fatalf("live err = %v", err)
	}
}

func TestLiveRefreshCooldownAnd429(t *testing.T) {
	calls := 0
	now := fixedNow
	r := newRemote(t, RemoteConfig{Tags: fixtureTags{&calls}, Cycles: fixtureCycles{}, Now: func() time.Time { return now }})
	view, cooled, err := r.LiveRefresh(context.Background(), "redis")
	if err != nil || cooled || view.Origin != CatalogSourceLive || calls != 1 {
		t.Fatalf("view=%+v cooled=%v err=%v calls=%d", view, cooled, err, calls)
	}
	if _, cooled, _ := r.LiveRefresh(context.Background(), "redis"); !cooled || calls != 1 {
		t.Fatalf("cooldown not applied, calls = %d", calls)
	}
	now = now.Add(11 * time.Minute)
	if _, cooled, _ := r.LiveRefresh(context.Background(), "redis"); cooled || calls != 2 {
		t.Fatalf("cooldown did not expire, calls = %d", calls)
	}
	if _, _, err := r.LiveRefresh(context.Background(), "dragonfly"); !errors.Is(err, ErrUnsupportedEngine) {
		t.Fatalf("err = %v", err)
	}
	limited := newRemote(t, RemoteConfig{Tags: rateLimitedTags{}, Cycles: fixtureCycles{}})
	_, _, err = limited.LiveRefresh(context.Background(), "postgres")
	var rl *dbcatalogbuild.RateLimitedError
	if !errors.As(err, &rl) || rl.RetryAfter != 45*time.Second {
		t.Fatalf("err = %v", err)
	}
	if v, _ := limited.Versions("postgres"); v.Origin != CatalogSourceEmbedded || len(v.Versions) == 0 {
		t.Fatalf("previous list lost: %+v", v)
	}
}

func TestStaleCacheFallsBackToEmbedded(t *testing.T) {
	priv, pub, _ := dbcatalogbuild.GenerateKey()
	body := sampleFile(t)
	sig, _ := dbcatalogbuild.Sign(priv, body)
	hits := 0
	srv := publishedServer(t, body, sig, &hits)
	defer srv.Close()
	now := fixedNow
	dir := t.TempDir()
	cfg := RemoteConfig{DataDir: dir, URL: srv.URL + "/databases.v1.json", PublicKey: pub, HTTP: srv.Client(), Now: func() time.Time { return now }}
	r := newRemote(t, cfg)
	if err := r.RefreshPublished(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(100 * 24 * time.Hour)
	if s := r.Status(); s.Source != CatalogSourceEmbedded {
		t.Fatalf("source = %s", s.Source)
	}
}

func TestFreshnessThresholds(t *testing.T) {
	cases := map[time.Duration]string{
		24 * time.Hour: FreshnessFresh, 10 * 24 * time.Hour: FreshnessOK,
		40 * 24 * time.Hour: FreshnessAging, 120 * 24 * time.Hour: FreshnessStale,
	}
	for age, want := range cases {
		if got := FreshnessOf(fixedNow.Add(-age), fixedNow); got != want {
			t.Errorf("age %s = %s, want %s", age, got, want)
		}
	}
}
