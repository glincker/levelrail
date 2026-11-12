package secretconn

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPathAllowedPrefixConfusionAndTraversal(t *testing.T) {
	p := Policy{AllowedPrefixes: []string{"team-a", "apps/web"}}
	for path, want := range map[string]bool{
		"team-a":            true,
		"team-a/db":         true,
		"team-ab":           false, // sibling that merely shares a string prefix
		"team-ab/db":        false,
		"team-a/../team-b":  false,
		"team-a/%2e%2e/b":   false,
		"team-a//db":        false,
		"team-a/./db":       false,
		"team-a\\..\\other": false,
		"apps":              false,
		"apps/web/x/y":      true,
		"Team-A/db":         false,
		"":                  false,
	} {
		if got := p.PathAllowed(path); got != want {
			t.Errorf("PathAllowed(%q) = %v, want %v", path, got, want)
		}
	}
	if !(Policy{}).PathAllowed("anything/at/all") {
		t.Error("an empty prefix list must not restrict")
	}
}

func TestNormalizePolicy(t *testing.T) {
	got, err := NormalizePolicy(Policy{AllowedPrefixes: []string{" /team-a/ ", "team-a", "b"}, AllowedEnvs: []string{"Prod", "prod", "staging"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.AllowedPrefixes) != 2 || got.AllowedPrefixes[0] != "b" || got.AllowedPrefixes[1] != "team-a" || len(got.AllowedEnvs) != 2 {
		t.Fatalf("normalized = %+v", got)
	}
	for _, bad := range []Policy{
		{AllowedPrefixes: []string{"a/../b"}}, {AllowedPrefixes: []string{".."}}, {AllowedPrefixes: []string{"a//b"}},
		{AllowedPrefixes: []string{"a#b"}}, {AllowedPrefixes: []string{""}}, {AllowedPrefixes: []string{"/"}}, {AllowedEnvs: []string{" "}},
	} {
		if _, err := NormalizePolicy(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v accepted, want ErrInvalid", bad)
		}
	}
}

func TestEnvAllowedFailsClosedOnEmptyName(t *testing.T) {
	p := Policy{AllowedEnvs: []string{"prod"}}
	if !p.EnvAllowed("PROD") || p.EnvAllowed("") || p.EnvAllowed("staging") {
		t.Fatal("env matching must be case-insensitive and refuse empty or unlisted names")
	}
}

func policyConn(t *testing.T, pol Input) (*Service, context.Context) {
	t.Helper()
	svc, _ := newSvc(storeVaultOff())
	svc.Build = func(Connection) secretsource.Provider { return secretsource.NewFake("c") }
	ctx := context.Background()
	in := Input{Type: TypeDoppler, Credential: "tok", Config: Config{Project: "p", Environment: "e"},
		AllowedPrefixes: pol.AllowedPrefixes, AllowedEnvs: pol.AllowedEnvs}
	if _, err := svc.Create(ctx, "root", "c", in); err != nil {
		t.Fatal(err)
	}
	return svc, ctx
}

func TestDelegationNeedsBothListsAndMatches(t *testing.T) {
	list := func(s ...string) *[]string { return &s }
	svc, ctx := policyConn(t, Input{AllowedPrefixes: list("team-a"), AllowedEnvs: list("prod")})
	if !svc.DelegatedRefAllowed(ctx, "c", "prod", "team-a/db") {
		t.Fatal("inside both lists must be allowed")
	}
	for _, c := range [][2]string{{"staging", "team-a/db"}, {"prod", "team-ab/db"}, {"", "team-a/db"}, {"prod", "team-a/../x"}} {
		if svc.DelegatedRefAllowed(ctx, "c", c[0], c[1]) {
			t.Errorf("env %q path %q must be refused", c[0], c[1])
		}
	}
	if svc.DelegatedRefAllowed(ctx, "missing", "prod", "team-a/db") {
		t.Fatal("unknown connection must be refused")
	}
	half, ctx2 := policyConn(t, Input{AllowedPrefixes: list("team-a")})
	if half.DelegatedRefAllowed(ctx2, "c", "prod", "team-a/db") {
		t.Fatal("a connection without an env list must not delegate")
	}
	none, ctx3 := policyConn(t, Input{})
	if none.DelegatedRefAllowed(ctx3, "c", "prod", "team-a/db") {
		t.Fatal("a connection without lists must not delegate")
	}
}

func TestCheckResolveFailsClosed(t *testing.T) {
	list := func(s ...string) *[]string { return &s }
	svc, ctx := policyConn(t, Input{AllowedPrefixes: list("team-a"), AllowedEnvs: list("prod")})
	if err := svc.CheckResolve(ctx, "c", "prod", "team-a/x"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckResolve(ctx, "c", "staging", "team-a/x"); !errors.Is(err, secretsource.ErrDenied) {
		t.Fatalf("wrong env = %v", err)
	}
	if err := svc.CheckResolve(ctx, "c", "prod", "team-ab/x"); !errors.Is(err, secretsource.ErrDenied) {
		t.Fatalf("prefix confusion = %v", err)
	}
	if err := svc.CheckResolve(ctx, "c", "", "team-a/x"); !errors.Is(err, secretsource.ErrDenied) {
		t.Fatalf("no environment = %v", err)
	}
	open, ctx2 := policyConn(t, Input{})
	if err := open.CheckResolve(ctx2, "c", "", "any/path"); err != nil {
		t.Fatalf("no policy must not restrict: %v", err)
	}
}

func TestPolicyOnlyUpdateKeepsSettingsAndCredential(t *testing.T) {
	list := func(s ...string) *[]string { return &s }
	svc, ctx := policyConn(t, Input{})
	c, err := svc.Update(ctx, "c", Input{AllowedPrefixes: list("team-a"), AllowedEnvs: list("prod")})
	if err != nil || len(c.Policy.AllowedPrefixes) != 1 || c.Config.Project != "p" || !c.HasCredential {
		t.Fatalf("update = %+v, %v", c, err)
	}
	c, err = svc.Update(ctx, "c", Input{AllowedEnvs: list()})
	if err != nil || len(c.Policy.AllowedEnvs) != 0 || len(c.Policy.AllowedPrefixes) != 1 {
		t.Fatalf("clearing one list must keep the other: %+v, %v", c, err)
	}
	if _, err := svc.Update(ctx, "c", Input{AllowedPrefixes: list("a/../b")}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("traversal prefix = %v", err)
	}
}

func storeVaultOff() store.VaultSettings { return store.VaultSettings{} }
