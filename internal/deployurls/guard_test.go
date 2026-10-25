package deployurls

import (
	"context"
	"testing"
)

type fakeGuardStore struct {
	apps      map[string]string
	suspended []string
}

func (f *fakeGuardStore) DeployURLApp(_ context.Context, name string) (string, bool, error) {
	app, ok := f.apps[name]
	return app, ok, nil
}

func (f *fakeGuardStore) UpdateServiceSuspended(_ context.Context, name string, s bool) error {
	if s {
		f.suspended = append(f.suspended, name)
	}
	return nil
}

type fakeSecrets map[string]string

func (f fakeSecrets) Exists(_ context.Context, svc, key string) (bool, error) {
	_, ok := f[svc+"/"+key]
	return ok, nil
}

func (f fakeSecrets) Resolve(_ context.Context, svc, key string) (string, error) {
	return f[svc+"/"+key], nil
}

func TestRetainedParentSecrets(t *testing.T) {
	st := &fakeGuardStore{apps: map[string]string{"web-abc1234": "web"}}
	r := ParentSecrets{Inner: fakeSecrets{"web/WHOAMI_PORT_NUMBER": "80"}, Apps: st}
	ctx := context.Background()
	if ok, _ := r.Exists(ctx, "web-abc1234", "WHOAMI_PORT_NUMBER"); !ok {
		t.Fatal("retained service must see its app's secret")
	}
	if v, _ := r.Resolve(ctx, "web-abc1234", "WHOAMI_PORT_NUMBER"); v != "80" {
		t.Fatalf("got %q, want the app's value", v)
	}
	if ok, _ := r.Exists(ctx, "other", "WHOAMI_PORT_NUMBER"); ok {
		t.Fatal("a service that is not retained must keep its own namespace")
	}
}

func TestRetainedGuardStopsAfterLimit(t *testing.T) {
	st := &fakeGuardStore{apps: map[string]string{"web-abc1234": "web"}}
	g := &Guard{Store: st, Max: 3}
	ctx := context.Background()
	for i := 1; i <= 2; i++ {
		if _, stopped, _ := g.NoteUnready(ctx, "web-abc1234"); stopped {
			t.Fatalf("stopped after %d failures, limit is 3", i)
		}
	}
	g.NoteReady("web-abc1234")
	for i := 1; i <= 2; i++ {
		if _, stopped, _ := g.NoteUnready(ctx, "web-abc1234"); stopped {
			t.Fatal("a ready pass must reset the count")
		}
	}
	if retained, stopped, _ := g.NoteUnready(ctx, "web-abc1234"); !retained || !stopped {
		t.Fatal("third consecutive failure must stop the service")
	}
	if len(st.suspended) != 1 || st.suspended[0] != "web-abc1234" {
		t.Fatalf("suspended = %v", st.suspended)
	}
	if retained, stopped, _ := g.NoteUnready(ctx, "web"); retained || stopped {
		t.Fatal("an ordinary app is never touched")
	}
}

func TestMaxUnreadyFromEnv(t *testing.T) {
	get := func(v string) func(string) string { return func(string) string { return v } }
	if MaxUnreadyFromEnv(get("")) != DefaultMaxUnready || MaxUnreadyFromEnv(get("x")) != DefaultMaxUnready || MaxUnreadyFromEnv(get("5")) != 5 {
		t.Fatal("env parsing")
	}
}
