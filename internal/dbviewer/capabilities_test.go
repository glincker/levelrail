package dbviewer_test

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/dbviewer"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCapabilitiesRegistryCoversEveryEngine(t *testing.T) {
	engines, err := store.SupportedDatabaseEngines()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range engines {
		row := dbviewer.CapabilitiesFor(e.ID)
		if row.Engine != e.ID || row.Kind == "" {
			t.Errorf("engine %q has no capability entry: %+v", e.ID, row)
		}
	}
	if got, want := len(dbviewer.CapabilityEngines()), len(engines); got != want {
		t.Errorf("registry has %d engines, yaml has %d", got, want)
	}
}

func TestCapabilitiesShapes(t *testing.T) {
	if c := dbviewer.CapabilitiesFor("postgres").Capabilities; c.Query.Status != dbviewer.StatusAvailable {
		t.Errorf("postgres query = %v", c.Query)
	}
	if c := dbviewer.CapabilitiesFor("redis").Capabilities; c.KeyBrowse.Status != dbviewer.StatusAvailable || c.Query.Status != dbviewer.StatusUnsupported {
		t.Errorf("redis caps = %+v", c)
	}
	m := dbviewer.CapabilitiesFor("mongodb")
	if !m.Planned || m.Capabilities.Documents.Status != dbviewer.StatusPlanned {
		t.Errorf("mongodb = %+v", m)
	}
}
