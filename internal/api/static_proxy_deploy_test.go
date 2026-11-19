package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploy"
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
	az := rt.proxyScopeAuthorizer("site")
	if scope, err := az(context.Background(), "same"); err != nil || scope != "t1/p1" {
		t.Fatalf("same scope: %q %v", scope, err)
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
