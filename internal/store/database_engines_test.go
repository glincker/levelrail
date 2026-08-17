package store

import "testing"

func TestSupportedDatabaseEngines_LoadsRealEmbeddedRegistry(t *testing.T) {
	engines, err := SupportedDatabaseEngines()
	if err != nil {
		t.Fatalf("SupportedDatabaseEngines() error = %v", err)
	}
	if len(engines) == 0 {
		t.Fatal("SupportedDatabaseEngines() returned no engines")
	}

	byID := make(map[string]DatabaseEngineInfo, len(engines))
	for _, e := range engines {
		if e.ID == "" {
			t.Errorf("engine %+v has an empty ID", e)
		}
		if e.Label == "" {
			t.Errorf("engine %q has an empty Label", e.ID)
		}
		if e.DefaultVersion == "" {
			t.Errorf("engine %q has an empty DefaultVersion", e.ID)
		}
		if _, dup := byID[e.ID]; dup {
			t.Errorf("duplicate engine id %q in registry", e.ID)
		}
		byID[e.ID] = e
	}

	for _, want := range []string{EnginePostgres, EngineRedis, EngineMySQL, EngineMongoDB} {
		if _, ok := byID[want]; !ok {
			t.Errorf("registry missing expected engine %q", want)
		}
	}
}

func TestIsSupportedEngine(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{EnginePostgres, true},
		{EngineRedis, true},
		{EngineMySQL, true},
		{EngineMongoDB, true},
		{"cassandra", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got, err := IsSupportedEngine(tt.id)
			if err != nil {
				t.Fatalf("IsSupportedEngine(%q) error = %v", tt.id, err)
			}
			if got != tt.want {
				t.Errorf("IsSupportedEngine(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

// TestSupportedEngines_MatchReconcilerCases is the drift guard the
// registry's own YAML comment promises: every engine the registry
// advertises must have a real case in
// internal/reconcile/database.Controller.Reconcile, or this control
// plane would let an operator create a database it can never actually
// start. Reconcile itself lives in a different package (this one would
// import a cycle back), so this test hand-maintains the same engine
// list Reconcile's switch statement uses and fails loudly if the two
// diverge, the cheapest cross-package check available without a shared
// interface neither package otherwise needs.
func TestSupportedEngines_MatchReconcilerCases(t *testing.T) {
	reconcilerHandles := map[string]bool{
		EnginePostgres: true,
		EngineRedis:    true,
		EngineMySQL:    true,
		EngineMongoDB:  true,
	}

	engines, err := SupportedDatabaseEngines()
	if err != nil {
		t.Fatalf("SupportedDatabaseEngines() error = %v", err)
	}
	for _, e := range engines {
		if !reconcilerHandles[e.ID] {
			t.Errorf("registry advertises engine %q with no matching case in internal/reconcile/database.Controller.Reconcile (update reconcilerHandles above once a real case is added)", e.ID)
		}
	}
	for id := range reconcilerHandles {
		found := false
		for _, e := range engines {
			if e.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("internal/reconcile/database.Controller.Reconcile handles engine %q but the registry doesn't advertise it", id)
		}
	}
}
