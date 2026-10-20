package sourcecache

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func seedMirror(t *testing.T, base, name string, size int, used time.Time) {
	t.Helper()
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pack"), make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, lastUsedFile)
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, used, used); err != nil {
		t.Fatal(err)
	}
}

func TestPrune(t *testing.T) {
	now := time.Now()
	day := 24 * time.Hour
	tests := []struct {
		name     string
		maxBytes int64
		maxAge   time.Duration
		busy     string
		want     []string
	}{
		{"nothing over limits", 1000, 30 * day, "", nil},
		{"age removes the stale one", 1000, 10 * day, "", []string{"c-old"}},
		{"size removes oldest first, name breaks ties", 150, 30 * day, "", []string{"c-old", "a-tie"}},
		{"busy mirror survives", 150, 10 * day, "c-old", []string{"a-tie", "b-tie"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			seedMirror(t, dir, "c-old", 100, now.Add(-20*day))
			seedMirror(t, dir, "a-tie", 100, now.Add(-2*day))
			seedMirror(t, dir, "b-tie", 100, now.Add(-2*day))
			c := New(Config{Dir: dir, MaxBytes: tt.maxBytes, MaxAge: tt.maxAge}, nil)
			if tt.busy != "" {
				c.stateFor(tt.busy).users = 1
			}
			got, err := c.Prune(now)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("removed %v, want %v", got, tt.want)
			}
		})
	}
}
