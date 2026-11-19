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
