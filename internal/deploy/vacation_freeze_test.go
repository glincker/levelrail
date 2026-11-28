package deploy

import (
	"context"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

type vacationStore struct{ ws []store.DeployFreezeWindow }

func (s vacationStore) ListDeployFreezeWindows(_ context.Context, scopes ...string) ([]store.DeployFreezeWindow, error) {
	var out []store.DeployFreezeWindow
	for _, w := range s.ws {
		for _, sc := range scopes {
			if w.Scope == sc {
				out = append(out, w)
			}
		}
	}
	return out, nil
}

func TestVacationFreezeHoldsThenLiftsItself(t *testing.T) {
	start := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	fs := vacationStore{ws: []store.DeployFreezeWindow{VacationWindow(start, 3, "away")}}
	ctx := context.Background()

	for _, tc := range []struct {
		name   string
		at     time.Time
		frozen bool
	}{
		{"before start", start.Add(-time.Minute), false},
		{"day two", start.Add(48 * time.Hour), true},
		{"after the last day", start.Add(72*time.Hour + time.Second), false},
	} {
		st, err := CheckFreeze(ctx, fs, "web", tc.at)
		if err != nil || st.Frozen != tc.frozen {
			t.Fatalf("%s: frozen=%v err=%v, want %v", tc.name, st.Frozen, err, tc.frozen)
		}
	}
	if err := ValidateVacationDays(0); err == nil {
		t.Fatal("0 days accepted")
	}
	if err := ValidateVacationDays(10); err != nil {
		t.Fatalf("10 days rejected: %v", err)
	}
}
