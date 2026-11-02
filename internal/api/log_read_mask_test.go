package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const readMaskSecret = "readmask-3b7d90c2-credential" //nolint:gosec // synthetic test value

// A value registered after a row was stored must not come back on the history read path the CLI uses.
func TestToLogEntryResourceMasksLateRegisteredSecret(t *testing.T) {
	secretmask.Add(readMaskSecret)
	e := telemetry.LogEntry{
		Timestamp: time.Now(), Stream: "stdout", Message: "connecting with " + readMaskSecret + " now",
		Structured: true, FieldsJSON: `{"msg":"ok","auth":"` + readMaskSecret + `"}`,
	}
	r := toLogEntryResource(e)
	if strings.Contains(r.Message, readMaskSecret) || strings.Contains(string(r.FieldsJSON), readMaskSecret) {
		t.Fatalf("secret survived: %q %s", r.Message, r.FieldsJSON)
	}
	if !strings.Contains(r.Message, "connecting with ") || !strings.Contains(string(r.FieldsJSON), `"msg":"ok"`) {
		t.Errorf("surrounding text lost: %q %s", r.Message, r.FieldsJSON)
	}
}

// Container output read straight from the runtime is the one path that skips the log store.
func TestReadContainerLogsMasksRuntimeOutput(t *testing.T) {
	secretmask.Add(readMaskSecret)
	rtm := &fakeLogsRuntime{lines: []string{"boot ok", "dial failed password=" + readMaskSecret + " refused"}}
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(func(string) (docker.Runtime, error) { return rtm, nil }))
	lines := rt.readContainerLogs(context.Background(), &store.DesiredService{Name: "web"}, time.Time{}, 10)
	if len(lines) != 2 {
		t.Fatalf("lines = %v", lines)
	}
	if strings.Contains(lines[1].Message, readMaskSecret) {
		t.Errorf("secret survived: %q", lines[1].Message)
	}
	if !strings.Contains(lines[1].Message, "dial failed password=") || !strings.Contains(lines[1].Message, " refused") || lines[0].Message != "boot ok" {
		t.Errorf("surrounding text lost: %v", lines)
	}
}
