package idgen

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
	}{
		{name: "underscore prefix", prefix: "rule_"},
		{name: "dash prefix", prefix: "inst-"},
		{name: "empty prefix", prefix: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := New(tt.prefix)
			if err != nil {
				t.Fatalf("New(%q) error = %v", tt.prefix, err)
			}
			if !strings.HasPrefix(id, tt.prefix) {
				t.Fatalf("New(%q) = %q, missing prefix", tt.prefix, id)
			}
			suffix := strings.TrimPrefix(id, tt.prefix)
			if len(suffix) != RandomBytes*2 {
				t.Fatalf("New(%q) suffix length = %d, want %d", tt.prefix, len(suffix), RandomBytes*2)
			}
			raw, err := hex.DecodeString(suffix)
			if err != nil {
				t.Fatalf("New(%q) suffix %q is not hex: %v", tt.prefix, suffix, err)
			}
			if len(raw) < 16 {
				t.Fatalf("New(%q) carries %d random bytes, want at least 16", tt.prefix, len(raw))
			}
		})
	}
}

func TestNew_Unique(t *testing.T) {
	const n = 10000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id, err := New("x_")
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("New() returned duplicate %q after %d calls", id, i)
		}
		seen[id] = struct{}{}
	}
}
