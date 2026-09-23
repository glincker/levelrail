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

// TestHumanBytes mirrors web/src/lib/format.ts's own formatBytes test
// cases: both sides must render the same value the same way regardless
// of which one produced it.
func TestHumanBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{name: "zero", bytes: 0, want: "0 B"},
		{name: "negative", bytes: -1, want: "0 B"},
		{name: "bytes", bytes: 512, want: "512 B"},
		{name: "kibibytes, no decimal at or above 10", bytes: 20 * 1024, want: "20 KiB"},
		{name: "mebibytes, one decimal below 10", bytes: int64(9.5 * 1024 * 1024), want: "9.5 MiB"},
		{name: "gibibytes, no decimal at or above 10", bytes: 22_492_737_536, want: "21 GiB"},
		{name: "tebibytes, no decimal at or above 10", bytes: 12 * 1024 * 1024 * 1024 * 1024, want: "12 TiB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HumanBytes(tt.bytes); got != tt.want {
				t.Errorf("HumanBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
