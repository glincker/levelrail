package store

import (
	"strings"
	"testing"
)

func TestNewStatusID(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
	}{
		{name: "component prefix", prefix: "stc_"},
		{name: "incident prefix", prefix: "sti_"},
		{name: "update prefix", prefix: "stu_"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := NewStatusID(tt.prefix)
			if err != nil {
				t.Fatalf("NewStatusID(%q) error = %v", tt.prefix, err)
			}
			if !strings.HasPrefix(a, tt.prefix) {
				t.Fatalf("NewStatusID(%q) = %q, missing prefix", tt.prefix, a)
			}
			b, err := NewStatusID(tt.prefix)
			if err != nil {
				t.Fatalf("NewStatusID(%q) error = %v", tt.prefix, err)
			}
			if a == b {
				t.Errorf("NewStatusID(%q) returned duplicate values: %v", tt.prefix, a)
			}
		})
	}
}
