package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

func TestMaskedSSEMasksAcrossLinesAndFlushesOnIdle(t *testing.T) {
	t.Setenv(EnvLogMaskIdleFlush, "20ms")
	secretmask.Add("ssemask-first-half\nssemask-second-half")
	rec := httptest.NewRecorder()
	m := newMaskedSSE(rec)

	m.Write(sseLogEvent{Line: "before", Stream: "stdout"})
	m.Write(sseLogEvent{Line: "k=ssemask-first-half", Stream: "stdout"})
	m.Write(sseLogEvent{Line: "ssemask-second-half after", Stream: "stdout"})
	m.Write(sseLogEvent{Line: "last line", Stream: "stdout"})

	select {
	case <-m.Idle():
		m.Flush()
	case <-time.After(2 * time.Second):
		t.Fatal("held tail was never flushed on idle")
	}
	body := rec.Body.String()
	if strings.Contains(body, "ssemask-first") || strings.Contains(body, "ssemask-second") {
		t.Fatalf("leak: %q", body)
	}
	for _, want := range []string{"before", "k=" + secretmask.Marker + " after", "last line"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %q", want, body)
		}
	}
	if m.Idle() != nil {
		t.Fatal("idle timer must be disarmed once nothing is held")
	}
}
