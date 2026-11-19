package ingress

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/staticroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

type scopeOwnersStore struct {
	ServiceStore
	owner *store.ResourceOwner
}

func (s scopeOwnersStore) OwnerOfApp(context.Context, string) (*store.ResourceOwner, error) {
	return s.owner, nil
}

func TestProxyScopeHolds(t *testing.T) {
	rules := &staticroutes.Ruleset{}
	rules.ProxyScopes = map[string]string{"api": "t1/p1"}
	cases := []struct {
		name  string
		store ServiceStore
		want  bool
	}{
		{"same scope", scopeOwnersStore{owner: &store.ResourceOwner{TeamID: "t1", ProjectID: "p1"}}, true},
		{"moved project", scopeOwnersStore{owner: &store.ResourceOwner{TeamID: "t1", ProjectID: "p2"}}, false},
		{"app gone", scopeOwnersStore{}, false},
	}
	for _, tc := range cases {
		c := &Controller{store: tc.store}
		if got := c.proxyScopeHolds(context.Background(), rules, "api"); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
	if (&Controller{store: scopeOwnersStore{}}).proxyScopeHolds(context.Background(), &staticroutes.Ruleset{}, "api") {
		t.Error("a ruleset with no recorded scope answers 502")
	}
}

type restrictedStore struct {
	ServiceStore
	basic []store.DomainBasicAuth
}

func (s restrictedStore) ListDomainBasicAuth(context.Context) ([]store.DomainBasicAuth, error) {
	return s.basic, nil
}

func (s restrictedStore) ListDomainTrafficPolicies(context.Context) ([]store.DomainTrafficPolicy, error) {
	return []store.DomainTrafficPolicy{{Domain: "geo.example.com", Kind: "geo"}, {Domain: "hdr.example.com", Kind: "headers"}}, nil
}

func TestProxyTargetRestrictedAtServeTime(t *testing.T) {
	services := []store.DesiredService{
		{Name: "plain", Domains: []string{"plain.example.com"}},
		{Name: "locked", Domains: []string{"locked.example.com"}},
		{Name: "geo", Domains: []string{"geo.example.com"}},
		{Name: "hdr", Domains: []string{"hdr.example.com"}},
		{Name: "internal"},
	}
	c := &Controller{store: restrictedStore{basic: []store.DomainBasicAuth{{Domain: "locked.example.com"}}}}
	for app, want := range map[string]bool{"plain": false, "locked": true, "geo": true, "hdr": false, "internal": false} {
		if got := c.proxyTargetRestricted(context.Background(), app, services); got != want {
			t.Errorf("%s: got %v want %v", app, got, want)
		}
	}
}
