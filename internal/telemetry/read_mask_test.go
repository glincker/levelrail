package telemetry

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

// The secret is registered after the rows were written, as happens when a
// user sets a new secret while old logs are still stored.
func TestQueryLogsAndDeployLogMaskSecretsRegisteredLater(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	secret := "late-registered-secret-4412" //nolint:gosec // synthetic test value
	now := time.Now().UTC()
	if err := db.WriteLogBatch(ctx, []LogEntry{{ResourceID: "svc", Stream: "stdout", Timestamp: now, Message: "auth " + secret + " ok"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.WriteDeployLogBatch(ctx, []DeployLogEntry{{AttemptID: "dep_m", Stream: "stdout", Timestamp: now, Message: "push " + secret}}); err != nil {
		t.Fatal(err)
	}
	secretmask.Add(secret)

	logs, err := db.QueryLogs(ctx, "svc", now.Add(-time.Minute), now.Add(time.Minute), "")
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs = %v, %v", logs, err)
	}
	if strings.Contains(logs[0].Message, secret) || !strings.Contains(logs[0].Message, "auth ") {
		t.Errorf("log message = %q", logs[0].Message)
	}
	dl, err := db.QueryDeployLog(ctx, "dep_m")
	if err != nil || len(dl) != 1 {
		t.Fatalf("deploy log = %v, %v", dl, err)
	}
	if strings.Contains(dl[0].Message, secret) || !strings.Contains(dl[0].Message, "push ") {
		t.Errorf("deploy log message = %q", dl[0].Message)
	}
}
