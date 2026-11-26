package api

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dbprovision"
	"github.com/GLINCKER/levelrail/internal/reconcile"
)

func crashCondition(msg string) []reconcile.Condition {
	return []reconcile.Condition{{Type: "Ready", Status: reconcile.ConditionFalse, Reason: dbprovision.ReasonCrashLoop, Message: msg}}
}

func TestSummarizeDatabaseConditionsCrashLoop(t *testing.T) {
	msg := "the database container exited 3 times within 1m0s; retrying in 10s. Last output: FATAL: database files are incompatible with server. The data directory was initialized by PostgreSQL version 18, which is not compatible with this version 19.0."
	s := summarizeDatabaseConditions(crashCondition(msg))
	if s.Label != "Crash looping" || s.Variant != "destructive" || s.Reason != dbprovision.ReasonCrashLoop {
		t.Fatalf("summary = %+v", s)
	}
	if !strings.Contains(s.Hint, "on disk: 18") || strings.Contains(s.Message, "exited 3 times") {
		t.Fatalf("hint %q message %q", s.Hint, s.Message)
	}
	if g := summarizeDatabaseConditions(crashCondition("exited. Last output: boom")); !strings.Contains(g.Hint, "logs") {
		t.Fatalf("generic hint = %q", g.Hint)
	}
}

func TestDatabaseCrashLoopDoctorResult(t *testing.T) {
	if r := databaseCrashLoopResult("c", "n", nil); r.Status != doctorStatusOK {
		t.Fatalf("empty = %+v", r)
	}
	r := databaseCrashLoopResult("c", "n", []crashLoopingDatabase{{Name: "main", Message: "boom", Hint: crashLoopHint("database files are incompatible with server")}})
	if r.Status != doctorStatusFail || !strings.Contains(r.Message, "main") || !strings.Contains(r.Fix, "another major version") {
		t.Fatalf("result = %+v", r)
	}
}
