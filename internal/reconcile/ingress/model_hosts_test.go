package ingress

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeModelHosts struct {
	hosts []string
	err   error
}

func (f fakeModelHosts) ModelHosts(context.Context) ([]string, error) { return f.hosts, f.err }

func TestController_Reconcile_ModelRoutes(t *testing.T) {
	hostOf := func(t *testing.T, applier *fakeApplier) []string {
		t.Helper()
		var out []string
		for _, r := range applier.routes(t) {
			for _, m := range r.Match {
				out = append(out, m.Host...)
			}
		}
		return out
	}

	t.Run("hosts route to the dashboard dial", func(t *testing.T) {
		applier := &fakeApplier{}
		c := New(&fakeStore{}, newFakeRuntime(), applier, WithLogger(discardLogger()),
			WithDashboardDial("127.0.0.1:8080"), WithModelHosts(fakeModelHosts{hosts: []string{"chat.example.com", "llm.example.com"}}))
		if _, err := c.Reconcile(context.Background()); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		got := hostOf(t, applier)
		if len(got) != 2 || got[0] != "chat.example.com" || got[1] != "llm.example.com" {
			t.Errorf("hosts = %v", got)
		}
	})

	t.Run("no dashboard dial means no model route", func(t *testing.T) {
		applier := &fakeApplier{}
		c := New(&fakeStore{}, newFakeRuntime(), applier, WithLogger(discardLogger()), WithModelHosts(fakeModelHosts{hosts: []string{"chat.example.com"}}))
		if _, err := c.Reconcile(context.Background()); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if got := hostOf(t, applier); len(got) != 0 {
			t.Errorf("hosts = %v, want none", got)
		}
	})

	t.Run("a host claimed by a service is not stolen", func(t *testing.T) {
		st := &fakeStore{settings: store.IngressSettings{PrimaryDomain: "chat.example.com"}}
		applier := &fakeApplier{}
		c := New(st, newFakeRuntime(), applier, WithLogger(discardLogger()),
			WithDashboardDial("127.0.0.1:8080"), WithModelHosts(fakeModelHosts{hosts: []string{"chat.example.com", "other.example.com"}}))
		if _, err := c.Reconcile(context.Background()); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		got := hostOf(t, applier)
		if len(got) != 2 {
			t.Errorf("hosts = %v, want the primary domain once plus other.example.com", got)
		}
	})

	t.Run("source error fails the pass", func(t *testing.T) {
		c := New(&fakeStore{}, newFakeRuntime(), &fakeApplier{}, WithLogger(discardLogger()),
			WithDashboardDial("127.0.0.1:8080"), WithModelHosts(fakeModelHosts{err: errors.New("db down")}))
		if _, err := c.Reconcile(context.Background()); err == nil {
			t.Fatal("want error")
		}
	})
}
