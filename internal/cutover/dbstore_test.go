package cutover

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDBStoreRoundTripAndSingleActiveRun(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := DBStore{DB: db}
	now := time.Now().UTC().Truncate(time.Second)
	run := Run{ID: "cut_1", SessionID: "s", SourceID: "src", App: "shop", Mode: ModeSwitch, State: StatePlanning, DNSWrite: true,
		Domains:   []DomainRun{{Domain: "shop.example.com", Method: MethodDNS, Previous: []Record{{Name: "shop", Type: "A", Value: "203.0.113.9", TTL: 300}}}},
		CreatedAt: now, UpdatedAt: now}
	if err := st.Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	second := run
	second.ID = "cut_2"
	if err := st.Create(ctx, second); !errors.Is(err, store.ErrCutoverRunActive) {
		t.Fatalf("second active run err = %v", err)
	}
	run.State, run.Plan = StateLive, &Plan{App: "shop", Verdict: VerdictReady}
	run.Steps = []Step{{Name: StepSwitch, State: StepDone, StartedAt: now, FinishedAt: now}}
	run.FinishedAt = now
	if err := st.Save(ctx, run); err != nil {
		t.Fatal(err)
	}
	got, err := st.Get(ctx, "cut_1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateLive || got.Plan == nil || got.Domains[0].Previous[0].Value != "203.0.113.9" || !got.DNSWrite || len(got.Steps) != 1 {
		t.Fatalf("round trip lost data: %+v", got)
	}
	if err := st.Create(ctx, second); err != nil {
		t.Fatalf("a live run must not block a new one: %v", err)
	}
	if _, err := st.Get(ctx, "missing"); !errors.Is(err, store.ErrCutoverRunNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	list, err := st.List(ctx, "shop", 10)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %d, %v", len(list), err)
	}
	active, err := st.ListByState(ctx, StatePlanning)
	if err != nil || len(active) != 1 || active[0].ID != "cut_2" {
		t.Fatalf("active = %+v, %v", active, err)
	}
}
