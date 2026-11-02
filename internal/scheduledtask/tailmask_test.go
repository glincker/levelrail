package scheduledtask

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

func TestTailCappedWriterNeverLeaksSecretAtCapBoundary(t *testing.T) {
	secret := "tailcap-secret-value-9876"
	secretmask.Add(secret)
	w := newTailCappedWriter(40)
	_, _ = w.Write([]byte(strings.Repeat("x", 100) + " key=" + secret[:10]))
	_, _ = w.Write([]byte(secret[10:] + " end-of-output-marker"))
	got := w.String()
	if strings.Contains(got, "tailcap") || strings.Contains(got, "9876") || strings.Contains(got, "secret-value") {
		t.Fatalf("leak: %q", got)
	}
	if !strings.Contains(got, "end-of-output-marker") {
		t.Fatalf("surrounding text lost: %q", got)
	}
}

func TestTailCappedWriterCutInsideSecretHead(t *testing.T) {
	secret := "headcut-secret-value-4321"
	secretmask.Add(secret)
	w := newTailCappedWriter(30)
	_, _ = w.Write([]byte("k=" + secret + strings.Repeat(".", 25)))
	got := w.String()
	if strings.Contains(got, "4321") || strings.Contains(got, "value") {
		t.Fatalf("fragment leaked: %q", got)
	}
}
