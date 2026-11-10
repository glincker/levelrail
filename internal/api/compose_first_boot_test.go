package api

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestApplyFirstBootTimeout(t *testing.T) {
	old := catalog.Templates
	t.Cleanup(func() { catalog.Templates = old })
	catalog.Templates = append(catalog.Templates[:len(catalog.Templates):len(catalog.Templates)],
		catalog.Template{ID: "first-boot-fixture", Manifest: &catalog.Manifest{FirstBootSeconds: 600}})
	t.Setenv("APP_TEMPLATE_FIRST_BOOT_MAX_SECONDS", "120")

	rt := &Router{}
	services := []store.DesiredService{
		{Name: "a"},
		{Name: "b", Health: &store.ServiceHealth{ReadyTimeout: 5 * time.Minute}},
	}
	rt.applyFirstBootTimeout(context.Background(), "first-boot-fixture", services)
	if got := services[0].Health.ReadyTimeout; got != 2*time.Minute {
		t.Errorf("unset budget = %v, want the 120s ceiling", got)
	}
	if got := services[1].Health.ReadyTimeout; got != 5*time.Minute {
		t.Errorf("larger compose budget = %v, want it kept", got)
	}

	other := []store.DesiredService{{Name: "c"}}
	rt.applyFirstBootTimeout(context.Background(), "not-a-template", other)
	if other[0].Health != nil {
		t.Errorf("unknown template changed health: %+v", other[0].Health)
	}
}
