package platformimport

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/platformimport/fakeheroku"
)

func TestHerokuDiscover(t *testing.T) {
	fake := fakeheroku.New()
	srv := httptest.NewServer(fake.Handler())
	defer srv.Close()
	src, err := NewHeroku(srv.URL, fakeheroku.Token, testOpts)
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
	if r := fake.Ranges(); len(r) != 1 || !strings.Contains(r[0], "max=1000") {
		t.Errorf("range header: %v", r)
	}
	// shop, its worker, and bare.
	if len(d.Apps) != 3 || d.Apps[0].Name != "shop" || d.Apps[1].Name != "shop-worker" || d.Apps[2].Name != "bare" {
		t.Fatalf("apps: %+v", d.Apps)
	}
	shop := d.Apps[0]
	if shop.Kind != SourceUnknown || shop.Replicas != 2 || shop.Project != "acme" || strings.Join(shop.Domains, ",") != "shop.example.com" {
		t.Errorf("shop: %+v", shop)
	}
	if len(shop.Notes) == 0 || !strings.Contains(shop.Notes[0].Manual, "import <repo>") {
		t.Errorf("want repo import note: %+v", shop.Notes)
	}
	if d.Apps[1].Replicas != 1 {
		t.Errorf("worker: %+v", d.Apps[1])
	}
	if len(d.Apps[2].Env) != 0 || d.Apps[2].Replicas != 1 {
		t.Errorf("bare: %+v", d.Apps[2])
	}
}

func TestHerokuSecretsMasked(t *testing.T) {
	srv := httptest.NewServer(fakeheroku.New().Handler())
	defer srv.Close()
	src, _ := NewHeroku(srv.URL, fakeheroku.Token, testOpts)
	d, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	shop := d.Apps[0]
	for _, k := range []string{"SESSION_SECRET", "DATABASE_URL", "REDIS_URL", "SENDGRID_URL"} {
		if e := envOf(shop, k); !e.Secret || e.Value == "" {
			t.Errorf("%s should be a secret with a value: %+v", k, e)
		}
	}
	if envOf(shop, "NODE_ENV").Secret {
		t.Error("NODE_ENV must not be secret")
	}
	for _, w := range d.Warnings {
		if strings.Contains(w, fakeheroku.SessionKey) || strings.Contains(w, "pw-from-addon") {
			t.Errorf("warning leaks a value: %s", w)
		}
	}
}

func TestHerokuAddonMapping(t *testing.T) {
	srv := httptest.NewServer(fakeheroku.New().Handler())
	defer srv.Close()
	src, _ := NewHeroku(srv.URL, fakeheroku.Token, testOpts)
	d, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Databases) != 2 || d.Databases[0].Engine != "postgres" || d.Databases[1].Engine != "redis" || d.Databases[0].Project != "shop" {
		t.Fatalf("databases: %+v", d.Databases)
	}
	all := strings.Join(d.Warnings, "\n")
	for _, want := range []string{"heroku-postgresql:standard-0", "heroku-redis:premium-0", "no managed equivalent, set up manually: sendgrid"} {
		if !strings.Contains(all, want) {
			t.Errorf("warnings miss %q:\n%s", want, all)
		}
	}
}

func TestHerokuEmptyAccount(t *testing.T) {
	fake := fakeheroku.New()
	fake.Empty = true
	srv := httptest.NewServer(fake.Handler())
	defer srv.Close()
	src, _ := NewHeroku(srv.URL, fakeheroku.Token, testOpts)
	d, err := src.Discover(context.Background())
	if err != nil || len(d.Apps) != 0 || len(d.Databases) != 0 || len(d.Warnings) != 0 {
		t.Fatalf("d=%+v err=%v", d, err)
	}
}

func TestHerokuUnauthorizedRedactsToken(t *testing.T) {
	srv := httptest.NewServer(fakeheroku.New().Handler())
	defer srv.Close()
	src, _ := NewHeroku(srv.URL, "wrong-token-fixture", testOpts)
	_, err := src.Discover(context.Background())
	var api *APIError
	if err == nil || !errors.As(err, &api) || api.Status != 401 || strings.Contains(err.Error(), "wrong-token-fixture") {
		t.Fatalf("want redacted 401, got %v", err)
	}
}

func TestHerokuPlatformWiring(t *testing.T) {
	if p, err := ParsePlatform("heroku"); err != nil || p != Heroku {
		t.Fatalf("ParsePlatform: %v %v", p, err)
	}
	if _, err := NewSource(Heroku, "", "tok", testOpts); err != nil {
		t.Errorf("empty url should use the public API: %v", err)
	}
}
