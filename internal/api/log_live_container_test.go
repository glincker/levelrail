package api

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCurrentContainerFilter_Admit(t *testing.T) {
	t.Run("nil lookup always shows, never transitions", func(t *testing.T) {
		f := newCurrentContainerFilter(context.Background(), nil, "svc")
		show, transitioned := f.admit(context.Background(), "any-id")
		if !show || transitioned {
			t.Fatalf("admit() = (%v, %v), want (true, false)", show, transitioned)
		}
	})

	t.Run("unresolvable lookup always shows", func(t *testing.T) {
		lookup := func(context.Context, string) ([]string, bool) { return nil, false }
		f := newCurrentContainerFilter(context.Background(), lookup, "svc")
		show, transitioned := f.admit(context.Background(), "any-id")
		if !show || transitioned {
			t.Fatalf("admit() = (%v, %v), want (true, false)", show, transitioned)
		}
	})

	t.Run("empty container ID always shows, even when resolvable", func(t *testing.T) {
		lookup := func(context.Context, string) ([]string, bool) { return []string{"c1"}, true }
		f := newCurrentContainerFilter(context.Background(), lookup, "svc")
		show, transitioned := f.admit(context.Background(), "")
		if !show || transitioned {
			t.Fatalf("admit(\"\") = (%v, %v), want (true, false)", show, transitioned)
		}
	})

	t.Run("already-known current container shows without a fresh lookup", func(t *testing.T) {
		calls := 0
		lookup := func(context.Context, string) ([]string, bool) {
			calls++
			return []string{"c1"}, true
		}
		f := newCurrentContainerFilter(context.Background(), lookup, "svc")
		if calls != 1 {
			t.Fatalf("construction calls = %d, want 1", calls)
		}
		show, transitioned := f.admit(context.Background(), "c1")
		if !show || transitioned {
			t.Fatalf("admit(c1) = (%v, %v), want (true, false)", show, transitioned)
		}
		if calls != 1 {
			t.Fatalf("admit on a known-current id triggered a fresh lookup: calls = %d, want 1", calls)
		}
	})

	t.Run("fresh lookup failure fails closed and does not poison cached state", func(t *testing.T) {
		ok := true
		lookup := func(context.Context, string) ([]string, bool) {
			if !ok {
				return nil, false
			}
			return []string{"c1"}, true
		}
		f := newCurrentContainerFilter(context.Background(), lookup, "svc")
		ok = false
		show, transitioned := f.admit(context.Background(), "c2")
		if show || transitioned {
			t.Fatalf("admit(c2) during a failed re-resolve = (%v, %v), want (false, false)", show, transitioned)
		}
		ok = true
		show, transitioned = f.admit(context.Background(), "c1")
		if !show || transitioned {
			t.Fatalf("admit(c1) after a failed re-resolve for a different id = (%v, %v), want (true, false)", show, transitioned)
		}
	})

	t.Run("unrecognized container is rejected and the negative result is cached", func(t *testing.T) {
		calls := 0
		lookup := func(context.Context, string) ([]string, bool) {
			calls++
			return []string{"c1"}, true
		}
		f := newCurrentContainerFilter(context.Background(), lookup, "svc")
		show, transitioned := f.admit(context.Background(), "dead")
		if show || transitioned {
			t.Fatalf("admit(dead) = (%v, %v), want (false, false)", show, transitioned)
		}
		callsAfterFirst := calls
		show, transitioned = f.admit(context.Background(), "dead")
		if show || transitioned {
			t.Fatalf("admit(dead) second call = (%v, %v), want (false, false)", show, transitioned)
		}
		if calls != callsAfterFirst {
			t.Fatalf("a cached negative result triggered another lookup: calls = %d, want %d", calls, callsAfterFirst)
		}
	})

	t.Run("a cutover to a new container shows it and reports transitioned", func(t *testing.T) {
		current := []string{"old"}
		lookup := func(context.Context, string) ([]string, bool) { return current, true }
		f := newCurrentContainerFilter(context.Background(), lookup, "svc")

		current = []string{"new"}
		show, transitioned := f.admit(context.Background(), "new")
		if !show || !transitioned {
			t.Fatalf("admit(new) after cutover = (%v, %v), want (true, true)", show, transitioned)
		}

		// The old id is no longer current: it must now be rejected, not
		// grandfathered in from the filter's original construction.
		show, transitioned = f.admit(context.Background(), "old")
		if show || transitioned {
			t.Fatalf("admit(old) after cutover = (%v, %v), want (false, false)", show, transitioned)
		}
	})

	t.Run("re-resolving to the same set is not a transition", func(t *testing.T) {
		lookup := func(context.Context, string) ([]string, bool) { return []string{"c1", "c2"}, true }
		f := newCurrentContainerFilter(context.Background(), lookup, "svc")
		// Force a fresh lookup via an unknown id, but the set it resolves
		// to is identical to what construction already saw.
		show, transitioned := f.admit(context.Background(), "c2")
		if !show || transitioned {
			t.Fatalf("admit(c2) = (%v, %v), want (true, false): re-resolving the same set must not report a transition", show, transitioned)
		}
	})
}

func TestToIDSet(t *testing.T) {
	s := toIDSet([]string{"a", "b", "a"})
	if len(s) != 2 {
		t.Fatalf("toIDSet dedup: len = %d, want 2", len(s))
	}
	if _, ok := s["a"]; !ok {
		t.Fatal(`toIDSet: "a" missing`)
	}
	if len(toIDSet(nil)) != 0 {
		t.Fatal("toIDSet(nil) should be empty, not nil-panicking or populated")
	}
}

func TestSameIDSet(t *testing.T) {
	cases := []struct {
		name string
		a, b map[string]struct{}
		want bool
	}{
		{"both empty", toIDSet(nil), toIDSet(nil), true},
		{"equal", toIDSet([]string{"a", "b"}), toIDSet([]string{"b", "a"}), true},
		{"different length", toIDSet([]string{"a"}), toIDSet([]string{"a", "b"}), false},
		{"same length, different members", toIDSet([]string{"a", "b"}), toIDSet([]string{"a", "c"}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameIDSet(tc.a, tc.b); got != tc.want {
				t.Fatalf("sameIDSet(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestRouter_CurrentAppContainerIDs(t *testing.T) {
	t.Run("no execRuntime configured", func(t *testing.T) {
		rt, db := newTestRouter(t)
		seedServiceForCurrentContainerTest(t, db)
		ids, ok := rt.currentAppContainerIDs(context.Background(), "web")
		if ok || ids != nil {
			t.Fatalf("currentAppContainerIDs() = (%v, %v), want (nil, false)", ids, ok)
		}
	})

	t.Run("unknown app", func(t *testing.T) {
		rt, _ := newTestRouterWithCurrentContainerRuntime(t, &fakeExecAppRuntime{})
		ids, ok := rt.currentAppContainerIDs(context.Background(), "does-not-exist")
		if ok || ids != nil {
			t.Fatalf("currentAppContainerIDs() for an unknown app = (%v, %v), want (nil, false)", ids, ok)
		}
	})

	t.Run("replica running", func(t *testing.T) {
		fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "live-id", Running: true}}
		rt, db := newTestRouterWithCurrentContainerRuntime(t, fake)
		seedServiceForCurrentContainerTest(t, db)
		ids, ok := rt.currentAppContainerIDs(context.Background(), "web")
		if !ok {
			t.Fatalf("currentAppContainerIDs() ok = false, want true")
		}
		if len(ids) != 1 || ids[0] != "live-id" {
			t.Fatalf("currentAppContainerIDs() ids = %v, want [\"live-id\"]", ids)
		}
	})

	t.Run("replica not running", func(t *testing.T) {
		fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "gone", Running: false}}
		rt, db := newTestRouterWithCurrentContainerRuntime(t, fake)
		seedServiceForCurrentContainerTest(t, db)
		ids, ok := rt.currentAppContainerIDs(context.Background(), "web")
		if !ok {
			t.Fatalf("currentAppContainerIDs() ok = false, want true")
		}
		if len(ids) != 0 {
			t.Fatalf("currentAppContainerIDs() ids = %v, want empty", ids)
		}
	})

	t.Run("inspect error is treated as not running, not a resolution failure", func(t *testing.T) {
		fake := &fakeExecAppRuntime{inspectErr: errors.New("daemon unreachable")}
		rt, db := newTestRouterWithCurrentContainerRuntime(t, fake)
		seedServiceForCurrentContainerTest(t, db)
		ids, ok := rt.currentAppContainerIDs(context.Background(), "web")
		if !ok {
			t.Fatalf("currentAppContainerIDs() ok = false, want true")
		}
		if len(ids) != 0 {
			t.Fatalf("currentAppContainerIDs() ids = %v, want empty", ids)
		}
	})
}

func TestRouter_CurrentDatabaseContainerIDs(t *testing.T) {
	t.Run("no execRuntime configured", func(t *testing.T) {
		rt, db := newTestRouter(t)
		seedDatabaseForCurrentContainerTest(t, db, "pg")
		ids, ok := rt.currentDatabaseContainerIDs(context.Background(), "pg")
		if ok || ids != nil {
			t.Fatalf("currentDatabaseContainerIDs() = (%v, %v), want (nil, false)", ids, ok)
		}
	})

	t.Run("unknown database", func(t *testing.T) {
		rt, _ := newTestRouterWithCurrentContainerRuntime(t, &fakeExecAppRuntime{})
		ids, ok := rt.currentDatabaseContainerIDs(context.Background(), "does-not-exist")
		if ok || ids != nil {
			t.Fatalf("currentDatabaseContainerIDs() for an unknown database = (%v, %v), want (nil, false)", ids, ok)
		}
	})

	t.Run("running", func(t *testing.T) {
		fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "pg-id", Running: true}}
		rt, db := newTestRouterWithCurrentContainerRuntime(t, fake)
		seedDatabaseForCurrentContainerTest(t, db, "pg")
		ids, ok := rt.currentDatabaseContainerIDs(context.Background(), "pg")
		if !ok {
			t.Fatalf("currentDatabaseContainerIDs() ok = false, want true")
		}
		if len(ids) != 1 || ids[0] != "pg-id" {
			t.Fatalf("currentDatabaseContainerIDs() ids = %v, want [\"pg-id\"]", ids)
		}
	})

	t.Run("not running", func(t *testing.T) {
		fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "pg-id", Running: false}}
		rt, db := newTestRouterWithCurrentContainerRuntime(t, fake)
		seedDatabaseForCurrentContainerTest(t, db, "pg")
		ids, ok := rt.currentDatabaseContainerIDs(context.Background(), "pg")
		if !ok {
			t.Fatalf("currentDatabaseContainerIDs() ok = false, want true")
		}
		if len(ids) != 0 {
			t.Fatalf("currentDatabaseContainerIDs() ids = %v, want empty", ids)
		}
	})
}

func newTestRouterWithCurrentContainerRuntime(t *testing.T, fake docker.Runtime) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	resolver := func(string) (docker.Runtime, error) { return fake, nil }
	return NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(resolver)), db
}

func seedServiceForCurrentContainerTest(t *testing.T, db *store.DB) {
	t.Helper()
	const name = "web"
	svc := store.DesiredService{Name: name, Image: "levelrail/web:1", Port: 3000}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatalf("SaveDesiredService(%q): %v", name, err)
	}
}

func seedDatabaseForCurrentContainerTest(t *testing.T, db *store.DB, name string) {
	t.Helper()
	d := store.DesiredDatabase{Name: name, Engine: "postgres", Version: "16"}
	if err := db.SaveDesiredDatabase(context.Background(), d); err != nil {
		t.Fatalf("SaveDesiredDatabase(%q): %v", name, err)
	}
}
