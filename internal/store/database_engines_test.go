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

	for _, want := range []string{
		EnginePostgres, EngineRedis, EngineMySQL, EngineMongoDB,
		EngineMariaDB, EngineKeyDB, EngineDragonfly, EngineClickHouse,
	} {
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
		{EngineMariaDB, true},
		{EngineKeyDB, true},
		{EngineDragonfly, true},
		{EngineClickHouse, true},
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

func TestSupportedDatabaseEngines_PostgresVariants(t *testing.T) {
	engines, err := SupportedDatabaseEngines()
	if err != nil {
		t.Fatalf("SupportedDatabaseEngines() error = %v", err)
	}
	var postgres DatabaseEngineInfo
	found := false
	for _, e := range engines {
		if e.ID == EnginePostgres {
			postgres = e
			found = true
			break
		}
	}
	if !found {
		t.Fatal("registry missing postgres entry")
	}
	if len(postgres.Variants) == 0 {
		t.Fatal("postgres registry entry has no variants")
	}
	byID := make(map[string]DatabaseEngineVariantInfo, len(postgres.Variants))
	for _, v := range postgres.Variants {
		if v.ID == "" || v.Label == "" || v.Image == "" || v.TagTemplate == "" {
			t.Errorf("variant %+v has an empty field", v)
		}
		byID[v.ID] = v
	}
	for _, want := range []string{"pgvector", "postgis", "timescaledb"} {
		if _, ok := byID[want]; !ok {
			t.Errorf("postgres registry missing expected variant %q", want)
		}
	}
}

func TestIsSupportedVariant(t *testing.T) {
	tests := []struct {
		name    string
		engine  string
		variant string
		want    bool
	}{
		{"empty variant always valid", EnginePostgres, "", true},
		{"empty variant valid for any engine", EngineRedis, "", true},
		{"pgvector valid for postgres", EnginePostgres, "pgvector", true},
		{"postgis valid for postgres", EnginePostgres, "postgis", true},
		{"timescaledb valid for postgres", EnginePostgres, "timescaledb", true},
		{"unknown variant invalid", EnginePostgres, "cassandra-flavor", false},
		{"postgres variant invalid for redis", EngineRedis, "pgvector", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsSupportedVariant(tt.engine, tt.variant)
			if err != nil {
				t.Fatalf("IsSupportedVariant(%q, %q) error = %v", tt.engine, tt.variant, err)
			}
			if got != tt.want {
				t.Errorf("IsSupportedVariant(%q, %q) = %v, want %v", tt.engine, tt.variant, got, tt.want)
			}
		})
	}
}

func TestDatabaseEngineVariant(t *testing.T) {
	v, err := DatabaseEngineVariant(EnginePostgres, "pgvector")
	if err != nil {
		t.Fatalf("DatabaseEngineVariant() error = %v", err)
	}
	if v.Image != "pgvector/pgvector" {
		t.Errorf("Image = %q, want %q", v.Image, "pgvector/pgvector")
	}
	if v.TagTemplate == "" {
		t.Error("TagTemplate is empty")
	}

	if _, err := DatabaseEngineVariant(EnginePostgres, "no-such-variant"); err == nil {
		t.Error("DatabaseEngineVariant() error = nil, want an error for an unknown variant")
	}
	if _, err := DatabaseEngineVariant("no-such-engine", "pgvector"); err == nil {
		t.Error("DatabaseEngineVariant() error = nil, want an error for an unknown engine")
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
		EnginePostgres:   true,
		EngineRedis:      true,
		EngineMySQL:      true,
		EngineMongoDB:    true,
		EngineMariaDB:    true,
		EngineKeyDB:      true,
		EngineDragonfly:  true,
		EngineClickHouse: true,
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
