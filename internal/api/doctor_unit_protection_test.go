package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
)

func TestDoctorUnitProtectionRowShape(t *testing.T) {
	rt := &Router{dataDir: t.TempDir()}
	got := rt.doctorCheckUnitProtection(time.Now())
	if got.Code != "unit_protection" || got.Status == "" {
		t.Fatalf("unexpected row: %+v", got)
	}
}

func TestUncleanRestartAttentionEmptyWithoutIncidents(t *testing.T) {
	rt := &Router{dataDir: t.TempDir()}
	if items := rt.uncleanRestartAttentionItems([]string{AbilityRead}, time.Now()); len(items) != 0 {
		t.Fatalf("want none, got %+v", items)
	}
	if got := attention.NewItem(attention.Warning, attention.KindUncleanRestart, "control-plane", "x"); got.Title == "" || got.Action == "" {
		t.Fatalf("kind info missing: %+v", got)
	}
}
