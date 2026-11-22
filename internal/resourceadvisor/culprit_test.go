package resourceadvisor

import (
	"testing"
	"time"
)

func TestMemoryCulpritNamesDominantApp(t *testing.T) {
	now := time.Now()
	mk := func(name string, v float64) Resource {
		return Resource{Ref: Ref{Kind: ResourceApp, Name: name}, Memory: []Sample{{At: now.Add(-time.Minute), Value: v}}}
	}
	name, share, ok := memoryCulprit([]Resource{mk("a", 100), mk("big", 900), mk("c", 100)}, now)
	if !ok || name != "big" || share < 0.8 {
		t.Fatalf("got %q %.2f %v", name, share, ok)
	}
	if _, _, ok := memoryCulprit([]Resource{mk("a", 100), mk("b", 100)}, now); ok {
		t.Fatal("even split must not name a culprit")
	}
}
