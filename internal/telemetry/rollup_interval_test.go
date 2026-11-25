package telemetry

import (
	"testing"
	"time"
)

func TestRefreshEveryChangesNextWait(t *testing.T) {
	tk := time.NewTicker(time.Hour)
	defer tk.Stop()
	cur := refreshEvery(tk, time.Hour, time.Hour)
	if cur != time.Hour {
		t.Fatalf("unchanged interval must stay, got %v", cur)
	}
	cur = refreshEvery(tk, cur, 20*time.Millisecond)
	if cur != 20*time.Millisecond {
		t.Fatalf("interval not updated: %v", cur)
	}
	select {
	case <-tk.C:
	case <-time.After(2 * time.Second):
		t.Fatal("ticker did not follow the new interval")
	}
	if got := refreshEvery(tk, cur, 0); got != cur {
		t.Fatalf("zero must keep the current interval, got %v", got)
	}
}
