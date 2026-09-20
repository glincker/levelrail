package diskspace

import "testing"

func TestFree(t *testing.T) {
	free, err := Free(t.TempDir())
	if err != nil {
		t.Fatalf("Free: %v", err)
	}
	if free <= 0 {
		t.Fatalf("Free: got %d, want > 0", free)
	}
}

func TestFree_MissingPath(t *testing.T) {
	if _, err := Free("/this/path/should/not/exist/on/any/machine"); err == nil {
		t.Fatal("Free: want error for a nonexistent path, got nil")
	}
}
