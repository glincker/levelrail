package pipeline

import "testing"

func TestCacheVolumeNameIsUnambiguous(t *testing.T) {
	e := &Engine{}
	e.cfg.NamePrefix = "lr"
	if e.cacheVolume("a-b", "c") == e.cacheVolume("a", "b-c") {
		t.Fatal("different app and key pairs must not share a volume name")
	}
	first, second := e.cacheVolume("a", "b"), e.cacheVolume("a", "b")
	if first != second {
		t.Fatal("the same pair must give a stable name")
	}
}
