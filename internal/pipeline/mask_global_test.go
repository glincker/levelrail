package pipeline

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

func TestMaskerLongestFirstAndGlobalRegistry(t *testing.T) {
	m := &masker{}
	m.add("abc")
	m.add("abcdef-long-job-secret")
	got := m.mask("v=abcdef-long-job-secret and abc")
	if strings.Contains(got, "long-job-secret") || strings.Contains(got, "def-") {
		t.Errorf("longer secret leaked a tail: %q", got)
	}
	if out := secretmask.Mask("x abcdef-long-job-secret"); strings.Contains(out, "long-job-secret") {
		t.Errorf("job secret not registered globally: %q", out)
	}
}
