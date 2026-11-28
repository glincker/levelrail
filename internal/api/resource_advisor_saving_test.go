package api

import (
	"testing"

	"github.com/glincker/levelrail/internal/costestimate"
	"github.com/glincker/levelrail/internal/resourceadvisor"
)

func TestIdleAppSaving(t *testing.T) {
	rates := costestimate.DefaultRateTable()
	idle := resourceadvisor.Finding{Kind: resourceadvisor.KindIdleApp}
	idle.Impact.SavingMemoryBytes = 2 << 30
	usd, provider := idleAppSaving(idle, rates)
	if usd <= 0 || provider == "" {
		t.Fatalf("idle app with 2 GiB: saving = %v, provider = %q", usd, provider)
	}
	other := idle
	other.Kind = resourceadvisor.KindMemoryOverProvisioned
	if usd, _ := idleAppSaving(other, rates); usd != 0 {
		t.Fatalf("non-idle finding priced: %v", usd)
	}
}
