package dbcatalogbuild

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixtureTags struct{ t *testing.T }

func (f fixtureTags) ListTags(_ context.Context, repo string) ([]string, error) {
	name := map[string]string{"library/postgres": "postgres.tags.json", "library/redis": "redis.tags.json"}[repo]
	if name == "" {
		return nil, errors.New("no fixture for " + repo)
	}
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		f.t.Fatal(err)
	}
	var d struct{ Tags []string }
	if err := json.Unmarshal(raw, &d); err != nil {
		f.t.Fatal(err)
	}
	return d.Tags, nil
}

func eolServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixtures := map[string]string{"/postgresql.json": "postgresql.eol.json", "/redis.json": "redis.eol.json"}
		name, ok := fixtures[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		raw, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw) //nolint:gosec // recorded fixture, not user input
	}))
}

func testBuilder(t *testing.T, srv *httptest.Server) *Builder {
	return &Builder{
		Tags:   fixtureTags{t},
		Cycles: EOLClient{HTTP: srv.Client(), Base: srv.URL},
		Now:    func() time.Time { return time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC) },
	}
}

func TestBuildPostgresFromFixtures(t *testing.T) {
	srv := eolServer(t)
	defer srv.Close()
	var spec Spec
	for _, s := range DefaultSpecs() {
		if s.ID == "postgres" {
			spec = s
		}
	}
	e, err := testBuilder(t, srv).BuildEngine(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	rec, byLine := 0, map[string]Version{}
	for _, v := range e.Versions {
		if v.Recommended {
			rec++
		}
		byLine[strings.SplitN(v.Version, ".", 2)[0]] = v
	}
	if rec != 1 {
		t.Fatalf("recommended count = %d", rec)
	}
	if v := byLine["15"]; v.Channel != ChannelEOL || v.EOL != "2027-11-11" {
		t.Fatalf("15 =%+v, want eol", v)
	}
	if v := byLine["17"]; v.Channel != ChannelStable || strings.Contains(v.Tag, "alpine") {
		t.Fatalf("17 = %+v", v)
	}
	if v := byLine["19rc1"]; v.Version != "" && v.Channel != ChannelRC && v.Channel != ChannelBeta {
		t.Fatalf("prerelease = %+v", v)
	}
	if e.Source.EOLURL != "https://endoflife.date/postgresql" {
		t.Fatalf("source = %+v", e.Source)
	}
}

func TestBuildAcceptFilterAndRedisLines(t *testing.T) {
	srv := eolServer(t)
	defer srv.Close()
	b := testBuilder(t, srv)
	b.Accept = func(_, v string) bool { return !strings.HasPrefix(v, "8.0") }
	var spec Spec
	for _, s := range DefaultSpecs() {
		if s.ID == "redis" {
			spec = s
		}
	}
	e, err := b.BuildEngine(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range e.Versions {
		if strings.HasPrefix(v.Version, "8.0") {
			t.Fatalf("accept filter ignored: %s", v.Version)
		}
	}
	if len(e.Versions) == 0 {
		t.Fatal("no versions")
	}
}

func TestValidateAgainst(t *testing.T) {
	mk := func(n int) File {
		vs := make([]Version, n)
		return File{Schema: SchemaVersion, Engines: map[string]Engine{"postgres": {Versions: vs}}}
	}
	prev := mk(9)
	cases := []struct {
		name string
		next File
		prev *File
		fail bool
	}{
		{"ok", mk(7), &prev, false},
		{"shrink", mk(5), &prev, true},
		{"empty", mk(0), nil, true},
		{"schema", File{Schema: 2, Engines: mk(1).Engines}, nil, true},
	}
	for _, c := range cases {
		if err := ValidateAgainst(c.next, c.prev); (err != nil) != c.fail {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
}

func TestSignRoundTripWithThrowawayKey(t *testing.T) {
	priv, pub, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"schema":1}`)
	sig, err := Sign(priv, data)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, sig, data); err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, sig, []byte("tampered")); err == nil {
		t.Fatal("tampered data verified")
	}
}

func TestEOLRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	_, err := EOLClient{HTTP: srv.Client(), Base: srv.URL}.Cycles(context.Background(), "redis")
	var rl *RateLimitedError
	if !errors.As(err, &rl) || rl.RetryAfter != 30*time.Second {
		t.Fatalf("err = %v", err)
	}
}
