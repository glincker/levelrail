package telemetry

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

func TestWriteErrorEventMasksRegisteredSecretInRaw(t *testing.T) {
	canary := "errevt-registered-value-77"
	secretmask.Add(canary)
	db := openErrDB(t)
	ctx := context.Background()
	raw := `{"message":"dial failed password=` + canary + ` after retry"}`
	ev := ErrorEvent{ResourceID: "service:web", Fingerprint: "fpm", Title: "Boom", Level: "error",
		EventID: "e1", TimestampNs: 1, Raw: raw}
	if _, err := db.WriteErrorEvent(ctx, ev, 3); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.QueryRowContext(ctx, `SELECT raw FROM error_events WHERE fingerprint = 'fpm'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, canary) || !strings.Contains(stored, "after retry") {
		t.Fatalf("raw = %q", stored)
	}
}
