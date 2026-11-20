package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/staticroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

type proxyAuthApps struct {
	AppStore
	have map[string]bool
}

func (a proxyAuthApps) GetDesiredService(_ context.Context, name string) (*store.DesiredService, error) {
	if !a.have[name] {
		return nil, errors.New("not found")
	}
	return &store.DesiredService{Name: name}, nil
}

type proxyLiveSites struct {
	StaticSiteStore
	dir string
}

func (p proxyLiveSites) GetStaticSite(context.Context, string) (*store.StaticSite, error) {
	return &store.StaticSite{RootDir: p.dir}, nil
}

type proxyAuthOwners struct {
	teamStore
	PolicyStore
	owners map[string]*store.ResourceOwner
}

func (o proxyAuthOwners) OwnerOfApp(_ context.Context, name string) (*store.ResourceOwner, error) {
	return o.owners[name], nil
}

func TestProxyScopeAuthorizer(t *testing.T) {
	own := func(team, project string) *store.ResourceOwner {
		return &store.ResourceOwner{TeamID: team, ProjectID: project}
	}
	rt := &Router{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rt.apps = proxyAuthApps{have: map[string]bool{"site": true, "same": true, "other": true, "orphan": true}}
	rt.policies = proxyAuthOwners{owners: map[string]*store.ResourceOwner{
		"site": own("t1", "p1"), "same": own("t1", "p1"), "other": own("t1", "p2"),
	}}
	dir := t.TempDir()
	live, err := staticroutes.Compile(staticroutes.Input{Redirects: "/a/* app://same/:splat 200\n/b/* app://other/:splat 200\n/c/* app://orphan/:splat 200\n"}, staticroutes.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := staticroutes.WriteSidecar(dir, live); err != nil {
		t.Fatal(err)
	}
	rt.staticSites = proxyLiveSites{dir: dir}
	live.ProxyScopes = map[string]string{"same": "t1/p1", "other": "t1/p1", "orphan": "t1/p1"}
	if err := staticroutes.WriteSidecar(dir, live); err != nil {
		t.Fatal(err)
	}
	az := rt.proxyScopeAuthorizer("site")
	if scope, err := az(context.Background(), "same"); err != nil || scope != "t1/p1" {
		t.Fatalf("same scope: %q %v", scope, err)
	}
	if _, err := az(context.Background(), "newapp"); err == nil {
		t.Fatal("an app absent from the live sidecar must not be introduced")
	}
	rt.apps = proxyAuthApps{have: map[string]bool{"site": true, "same": true, "other": true, "orphan": true, "fresh": true}}
	rt.policies = proxyAuthOwners{owners: map[string]*store.ResourceOwner{
		"site": own("t1", "p1"), "same": own("t1", "p1"), "other": own("t1", "p2"), "fresh": own("t1", "p1"),
	}}
	if _, err := az(context.Background(), "fresh"); !errors.Is(err, deploy.ErrProxyTargetNew) {
		t.Fatalf("a new target on a non-interactive path must be refused, got %v", err)
	}
	for _, app := range []string{"other", "orphan", "missing", "site"} {
		if _, err := az(context.Background(), app); !errors.Is(err, deploy.ErrProxyTargetDenied) {
			t.Errorf("%s must be denied with the shared error, got %v", app, err)
		}
	}
}

func TestScoreChecksCountsProxyWarnings(t *testing.T) {
	base := []staticSiteCheck{{ID: "x", Status: checkOK}}
	if scoreChecks(base) != 100 || scoreChecks(append(base, staticSiteCheck{ID: "proxy-target", Status: checkWarn})) >= 100 {
		t.Fatal("a proxy-target warning must lower the score")
	}
}

func TestRecordProxyScopesNeverWidens(t *testing.T) {
	rt := &Router{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rt.policies = proxyAuthOwners{owners: map[string]*store.ResourceOwner{
		"kept": {TeamID: "t1", ProjectID: "p1"}, "moved": {TeamID: "t2", ProjectID: "p9"},
		"fresh": {TeamID: "t1", ProjectID: "p1"}, "unapproved": {TeamID: "t1", ProjectID: "p1"},
	}}
	rs, err := staticroutes.Compile(staticroutes.Input{Redirects: "/a/* app://kept/:splat 200\n/b/* app://moved/:splat 200\n/c/* app://fresh/:splat 200\n/d/* app://unapproved/:splat 200\n"}, staticroutes.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	rs.ProxyScopes = map[string]string{"kept": "t1/p1", "moved": "t1/p1"}
	if err := rt.recordProxyScopes(context.Background(), rs, map[string]bool{"fresh": true, "moved": true}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"kept": "t1/p1", "fresh": "t1/p1"}
	if len(rs.ProxyScopes) != len(want) {
		t.Fatalf("scopes %v want %v", rs.ProxyScopes, want)
	}
	for k, v := range want {
		if rs.ProxyScopes[k] != v {
			t.Errorf("%s: %q want %q", k, rs.ProxyScopes[k], v)
		}
	}
}
