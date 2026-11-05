package telemetry

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

func TestMaskEntryHidesSecretBeforeFanOut(t *testing.T) {
	const canary = "drain-canary-secret-9917" //nolint:gosec // synthetic test value
	secretmask.Add(canary)
	b := NewLogBroadcaster()
	ch, unsub := b.Subscribe("service:x")
	defer unsub()

	b.Publish(maskEntry(LogEntry{ResourceID: "service:x", Message: "key=" + canary, FieldsJSON: `{"k":"` + canary + `"}`}))

	got := <-ch
	if strings.Contains(got.Message, canary) || strings.Contains(got.FieldsJSON, canary) {
		t.Errorf("published entry carries the secret: %+v", got)
	}
}
