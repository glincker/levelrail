package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
)

const refCanary = "CANARY_ref_3c91_value_77ab20ee" //nolint:gosec // fake canary

func refController(t *testing.T, fake *secretsource.FakeProvider, desired *store.DesiredService) (*Controller, *fakeRuntime) {
	t.Helper()
	res := secretsource.NewResolver()
	if err := res.Register("prod", fake); err != nil {
		t.Fatal(err)
	}
	rt := newFakeRuntime(0)
	c := New("web", &fakeStore{svc: desired}, rt, WithSecretSources(func() SecretSources { return testRefSources{res} }))
	return c, rt
}

func TestSecretRefResolvesAtCreateAndNeverLeaks(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(secretmask.Handler(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	t.Cleanup(func() { slog.SetDefault(prev) })

	fake := secretsource.NewFake("prod").Set("apps/web", "api", refCanary)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80,
		VaultEnv: map[string]store.VaultEnvRef{"API": {Conn: "prod", Path: "apps/web", Key: "api"}}}
	c, rt := refController(t, fake, desired)
	result, err := c.Reconcile(context.Background())
	if err != nil || rt.lastCreateEnv["API"] != refCanary {
		t.Fatalf("value must reach the container, err=%v", err)
	}
	cond, _ := json.Marshal(result)
	stored, _ := json.Marshal(desired)
	for i, s := range []string{string(cond), string(stored), logs.String(), secretmask.Mask("x " + refCanary)} {
		if strings.Contains(s, refCanary) {
			t.Fatalf("canary leaked on surface %d", i)
		}
	}
}

func TestSecretRefFailsClosed(t *testing.T) {
	fake := secretsource.NewFake("prod").Set("apps/web", "api", refCanary)
	fake.Deny("apps/web")
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80,
		VaultEnv: map[string]store.VaultEnvRef{"API": {Conn: "prod", Path: "apps/web", Key: "api"}}}
	c, rt := refController(t, fake, desired)
	_, err := c.Reconcile(context.Background())
	if err == nil || !errors.Is(err, secretsource.ErrDenied) {
		t.Fatalf("denied must block the deploy, err=%v", err)
	}
	if !strings.Contains(err.Error(), "secret://prod/apps/web#api") || strings.Contains(err.Error(), refCanary) {
		t.Fatalf("error must name the ref and never the value: %v", err)
	}
	if rt.lastCreateEnv != nil {
		t.Fatal("no container may be created")
	}
}

func TestSecretRefUnconfiguredFailsClosed(t *testing.T) {
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80,
		VaultEnv: map[string]store.VaultEnvRef{"API": {Conn: "prod", Path: "a", Key: "k"}}}
	c := New("web", &fakeStore{svc: desired}, newFakeRuntime(0))
	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("a reference with no connections must fail")
	}
}

type testRefSources struct{ r *secretsource.Resolver }

func (a testRefSources) ResolveRef(ctx context.Context, conn, path, key string, version int) (string, error) {
	v, err := a.r.Resolve(ctx, secretsource.Ref{Conn: conn, Path: path, Key: key, Version: version})
	if err != nil {
		return "", err
	}
	return string(v.Reveal()), nil
}

type guardedRefSources struct {
	testRefSources
	err        error
	unvettedOK bool
	got        string
}

func (g *guardedRefSources) CheckUnvettedRef(context.Context, string, string, string) error {
	if g.unvettedOK {
		return nil
	}
	return secretsource.ErrDenied
}

func (g *guardedRefSources) CheckRef(_ context.Context, envID, conn, path string) error {
	g.got = envID + "|" + conn + "|" + path
	return g.err
}

func TestSecretRefGuardBlocksBeforeAnyRead(t *testing.T) {
	fake := secretsource.NewFake("prod").Set("apps/web", "api", refCanary)
	res := secretsource.NewResolver()
	if err := res.Register("prod", fake); err != nil {
		t.Fatal(err)
	}
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, EnvironmentID: "env-1",
		VaultEnv: map[string]store.VaultEnvRef{"API": {Conn: "prod", Path: "apps/web", Key: "api"}}}
	g := &guardedRefSources{testRefSources: testRefSources{res}, err: secretsource.ErrDenied}
	rt := newFakeRuntime(0)
	c := New("web", &fakeStore{svc: desired}, rt, WithSecretSources(func() SecretSources { return g }))
	_, err := c.Reconcile(context.Background())
	if err == nil || !errors.Is(err, secretsource.ErrDenied) {
		t.Fatalf("a denying guard must block the container, got %v", err)
	}
	if g.got != "env-1|prod|apps/web" {
		t.Fatalf("guard saw %q", g.got)
	}
	if rt.lastCreateEnv["API"] != "" || strings.Contains(err.Error(), refCanary) {
		t.Fatal("no value may be read or shown when the guard denies")
	}
}

// A reference written by the deploy pipeline has no vetting actor, so only a delegating connection serves it.
func TestUnvettedSecretRefBlockedUnlessConnectionDelegates(t *testing.T) {
	fake := secretsource.NewFake("prod").Set("apps/web", "api", refCanary)
	res := secretsource.NewResolver()
	if err := res.Register("prod", fake); err != nil {
		t.Fatal(err)
	}
	ref := store.VaultEnvRef{Conn: "prod", Path: "apps/web", Key: "api", Unvetted: true}
	for _, ok := range []bool{false, true} {
		desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, EnvironmentID: "env-1",
			VaultEnv: map[string]store.VaultEnvRef{"API": ref}}
		g := &guardedRefSources{testRefSources: testRefSources{res}, unvettedOK: ok}
		rt := newFakeRuntime(0)
		c := New("web", &fakeStore{svc: desired}, rt, WithSecretSources(func() SecretSources { return g }))
		_, err := c.Reconcile(context.Background())
		if ok != (err == nil) {
			t.Fatalf("delegating=%v err=%v", ok, err)
		}
		if !ok && rt.lastCreateEnv["API"] != "" {
			t.Fatal("no value may be read for a denied unvetted reference")
		}
	}
}
