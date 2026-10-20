package secretmask_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const seededSecret = "pw-9f8e7d6c5b4a-seeded" //nolint:gosec // synthetic test value

func rawContains(t *testing.T, db *sql.DB, query string) bool {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	defer func() { _ = rows.Close() }()
	cols, _ := rows.Columns()
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(fmt.Sprint(vals...), seededSecret) {
			return true
		}
	}
	return false
}

func TestSeededSecretNeverReachesAnyWriteChannel(t *testing.T) {
	secretmask.Add(seededSecret)
	ctx := context.Background()

	sdb, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	e := store.AuditEntry{
		ID: "aud_x", ActorType: "session", ActorID: "u", ActorName: "n", Ability: "write", Method: "POST",
		Path: "/api/v1/apps/a/env?v=" + seededSecret, StatusCode: 200, RemoteAddr: "127.0.0.1",
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: "api", Detail: `{"value":"` + seededSecret + `"}`,
	}
	if err := sdb.SaveAuditEntry(ctx, e); err != nil {
		t.Fatal(err)
	}
	if rawContains(t, sdb.DB, `SELECT * FROM audit_log`) {
		t.Error("secret reached audit_log")
	}

	tdb, err := telemetry.Open(ctx, filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tdb.Close() })
	if err := tdb.WriteDeployLogBatch(ctx, []telemetry.DeployLogEntry{{AttemptID: "d1", Stream: "stdout", Timestamp: time.Now(), Message: "export TOKEN=" + seededSecret}}); err != nil {
		t.Fatal(err)
	}
	if rawContains(t, tdb.DB, `SELECT * FROM deploy_logs`) {
		t.Error("secret reached deploy_logs (build output)")
	}
	if err := tdb.WriteLogBatch(ctx, []telemetry.LogEntry{{ResourceID: "r", ContainerID: "c", Stream: "stdout", Timestamp: time.Now(), Message: "boot " + seededSecret, Structured: true, FieldsJSON: `{"k":"` + seededSecret + `"}`}}); err != nil {
		t.Fatal(err)
	}
	if rawContains(t, tdb.DB, `SELECT * FROM log_entries`) {
		t.Error("secret reached log_entries")
	}

	var buf bytes.Buffer
	slog.New(secretmask.Handler(slog.NewTextHandler(&buf, nil))).Error("failed with "+seededSecret, slog.String("k", seededSecret))
	if strings.Contains(buf.String(), seededSecret) {
		t.Error("secret reached the process log")
	}
}
