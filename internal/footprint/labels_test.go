package footprint

import (
	"context"
	"testing"
)

func TestGoroutineLabelsCountAndRelease(t *testing.T) {
	release := Attribute("test-stream")
	release2 := Attribute("test-stream")
	var got int64
	for _, s := range Subsystems() {
		if s.Name == "test-stream" {
			got = s.Count
		}
	}
	if got != 2 {
		t.Fatalf("count = %d", got)
	}
	release()
	release2()
	for _, s := range Subsystems() {
		if s.Name == "test-stream" {
			t.Fatalf("still listed: %d", s.Count)
		}
	}
}

func TestGoroutineLabelsRunHoldsCount(t *testing.T) {
	Run(context.Background(), "test-run", func(context.Context) {
		if len(Subsystems()) == 0 {
			t.Error("expected a live subsystem")
		}
	})
}
