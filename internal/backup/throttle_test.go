package backup

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func TestThrottledReaderPacesReads(t *testing.T) {
	var slept time.Duration
	r := newThrottledReader(context.Background(), bytes.NewReader(make([]byte, 4096)), 1).(*throttledReader)
	r.sleep = func(_ context.Context, d time.Duration) error { slept = d; return nil }
	if _, err := io.ReadAll(r); err != nil {
		t.Fatal(err)
	}
	if slept < 3*time.Second {
		t.Fatalf("4 KiB at 1 KB/s should wait about 4s, waited %v", slept)
	}
	if _, ok := newThrottledReader(context.Background(), bytes.NewReader(nil), 0).(*throttledReader); ok {
		t.Fatal("limit 0 must not wrap the reader")
	}
}

func TestSchedulerRunWindow(t *testing.T) {
	s := &Scheduler{RunWindow: func(context.Context) string { return "22:00-05:00" }}
	at := func(h int) time.Time { return time.Date(2026, 1, 2, h, 30, 0, 0, time.UTC) }
	for h, want := range map[int]bool{23: true, 2: true, 12: false, 5: false} {
		if got := s.windowOpen(context.Background(), at(h)); got != want {
			t.Fatalf("hour %d: got %v want %v", h, got, want)
		}
	}
	if _, _, _, err := runtimesettings.ParseBackupWindow("25:00-01:00"); err == nil {
		t.Fatal("bad window must fail")
	}
}
