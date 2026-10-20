package cpbackup

import (
	"context"
	"testing"
	"time"
)

func TestManager_Newest(t *testing.T) {
	ctx := context.Background()
	m := NewManager(openDB(t), t.TempDir())
	if _, ok, err := m.Newest(); err != nil || ok {
		t.Fatalf("empty dir: ok=%v err=%v, want false, nil", ok, err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tick := 0
	m.now = func() time.Time { tick++; return base.Add(time.Duration(tick) * time.Hour) }
	for i := 0; i < 2; i++ {
		if _, err := m.Create(ctx); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	got, ok, err := m.Newest()
	if err != nil || !ok || !got.Equal(base.Add(2*time.Hour)) {
		t.Fatalf("Newest = %v, %v, %v; want %v", got, ok, err, base.Add(2*time.Hour))
	}
}
