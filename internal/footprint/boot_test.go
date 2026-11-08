package footprint

import (
	"errors"
	"testing"
	"time"
)

func TestBootStagesRecordAndBound(t *testing.T) {
	resetBootForTest()
	want := errors.New("boom")
	if err := TimeBootStage("store", func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("error not passed through: %v", err)
	}
	RecordBootStage("docker", 5*time.Millisecond)
	got := BootStages()
	if len(got) != 2 || got[0].Name != "store" || got[1].Name != "docker" || got[1].DurationMS != 5 {
		t.Fatalf("stages = %+v", got)
	}
	if BootTotal() < 5*time.Millisecond {
		t.Fatalf("total = %v", BootTotal())
	}
	for i := 0; i < maxBootStages*2; i++ {
		RecordBootStage("x", time.Millisecond)
	}
	if n := len(BootStages()); n != maxBootStages {
		t.Fatalf("stages not bounded: %d", n)
	}
	got[0].Name = "mutated"
	if BootStages()[0].Name == "mutated" {
		t.Fatal("BootStages must return a copy")
	}
}
