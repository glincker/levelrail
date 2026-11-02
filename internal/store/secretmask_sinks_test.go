package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

const (
	sinkSecret = "sinkcanary-7c1e9a40-token" //nolint:gosec // synthetic test value
	sinkPEM    = "-----BEGIN CANARY KEY-----\nc2lua2NhbmFyeS1ib2R5LWxpbmUtb25l\nc2lua2NhbmFyeS1ib2R5LWxpbmUtdHdv\n-----END CANARY KEY-----"
)

// hasAll fails when the secret is still in got or the surrounding text did not survive.
func hasAll(t *testing.T, what, got, secret string, keep ...string) {
	t.Helper()
	if strings.Contains(got, secret) {
		t.Errorf("%s: secret survived in %q", what, got)
	}
	for _, k := range keep {
		if !strings.Contains(got, k) {
			t.Errorf("%s: %q missing from %q", what, k, got)
		}
	}
}

func TestAIChatRowsMaskRegisteredSecrets(t *testing.T) {
	secretmask.Add(sinkSecret)
	secretmask.Add(sinkPEM)
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	if _, err := db.CreateAIChatSession(ctx, "aisess_1", "user:a", now); err != nil {
		t.Fatal(err)
	}
	args := json.RawMessage(`{"value":"` + sinkSecret + `","note":"keep-args"}`)
	err := db.SaveAIChatMessage(ctx, AIChatMessage{
		ID: "aimsg_1", SessionID: "aisess_1", Role: "user", CreatedAt: now,
		Content:   "my key is " + sinkSecret + " and\n" + sinkPEM + "\nthanks",
		ToolCalls: []AIChatToolCall{{ID: "t1", Name: "set_env", Arguments: args, Status: "pending"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := db.ListAIChatMessages(ctx, "aisess_1")
	if err != nil || len(msgs) != 1 {
		t.Fatalf("list = %v, %v", msgs, err)
	}
	hasAll(t, "content", msgs[0].Content, sinkSecret, "my key is ", "thanks")
	hasAll(t, "content pem", msgs[0].Content, "c2lua2NhbmFyeS1ib2R5", "and\n")
	raw, _ := json.Marshal(msgs[0].ToolCalls)
	if !json.Valid(raw) {
		t.Fatalf("tool calls no longer valid JSON: %s", raw)
	}
	hasAll(t, "tool calls", string(raw), sinkSecret, "keep-args", "set_env")

	upd := []AIChatToolCall{{ID: "t1", Name: "set_env", Arguments: args, Status: "approved"}}
	if err := db.UpdateAIChatMessageToolCalls(ctx, "aimsg_1", upd); err != nil {
		t.Fatal(err)
	}
	m, _ := db.GetAIChatMessage(ctx, "aimsg_1")
	raw, _ = json.Marshal(m.ToolCalls)
	hasAll(t, "updated tool calls", string(raw), sinkSecret, "approved")

	if err := db.SaveAIChatConfirmation(ctx, AIChatConfirmation{
		ID: "aiconf_1", SessionID: "aisess_1", MessageID: "aimsg_1", ToolUseID: "t1", ToolName: "set_env",
		ToolInput: args, Status: "pending", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	// Pending: approving executes the stored input, so it must stay real.
	pending, err := db.GetAIChatConfirmation(ctx, "aiconf_1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pending.ToolInput), sinkSecret) {
		t.Fatalf("pending input lost the real value: %s", pending.ToolInput)
	}
	hasAll(t, "pending display", string(pending.ForDisplay().ToolInput), sinkSecret, "keep-args")

	result := json.RawMessage(`{"echo":"` + sinkSecret + `","ok":true}`)
	if err := db.ResolveAIChatConfirmation(ctx, "aiconf_1", "approved", result, now); err != nil {
		t.Fatal(err)
	}
	c, err := db.GetAIChatConfirmation(ctx, "aiconf_1")
	if err != nil {
		t.Fatal(err)
	}
	hasAll(t, "confirmation input", string(c.ToolInput), sinkSecret, "keep-args")
	hasAll(t, "confirmation result", string(c.Result), sinkSecret, `"ok":true`)
	if !json.Valid(c.Result) || !json.Valid(c.ToolInput) {
		t.Error("masked confirmation JSON is no longer valid")
	}
}

func TestGateAndDeployFailureRowsMaskRegisteredSecrets(t *testing.T) {
	secretmask.Add(sinkSecret)
	db := openTestDB(t)
	ctx := context.Background()
	saveSucceededAttempt(t, db, "dep_s", "web", "app:v1", time.Now())
	probe := "readiness probe GET /health?key=" + sinkSecret + " returned 500"
	if err := db.RecordGateEvent(ctx, DeployGateEvent{DeployAttemptID: "dep_s", ServiceName: "web", Phase: "readiness", Status: "failed", Reason: "probe", Message: probe}); err != nil {
		t.Fatal(err)
	}
	evs, err := db.ListGateEvents(ctx, "dep_s")
	if err != nil || len(evs) != 1 {
		t.Fatalf("events = %v, %v", evs, err)
	}
	hasAll(t, "gate message", evs[0].Message, sinkSecret, "readiness probe GET /health?key=", "returned 500")

	if _, err := db.FailDeployAttemptWithReason(ctx, "dep_s", "Crash", "exit with "+sinkSecret+" in env"); err != nil {
		t.Fatal(err)
	}
	a, _ := db.GetDeployAttempt(ctx, "dep_s")
	hasAll(t, "attempt error", a.Error, sinkSecret, "exit with ", " in env")
}

func TestDatabaseQueryHistoryMasksRegisteredSecrets(t *testing.T) {
	secretmask.Add(sinkSecret)
	secretmask.Add(sinkPEM)
	db := openTestDB(t)
	ctx := context.Background()
	sqlText := "INSERT INTO creds (k, pem) VALUES ('" + sinkSecret + "', '" + sinkPEM + "')"
	_, err := db.AddDatabaseQueryHistory(ctx, DatabaseQueryHistory{
		DatabaseName: "main", PrincipalType: "user", PrincipalID: "u1", Mode: "write", SQL: sqlText,
		Error: `duplicate key value "` + sinkSecret + `" violates constraint`,
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListDatabaseQueryHistory(ctx, "main", "user", "u1", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("history = %v, %v", rows, err)
	}
	hasAll(t, "sql", rows[0].SQL, sinkSecret, "INSERT INTO creds (k, pem) VALUES ('", "')")
	hasAll(t, "sql pem", rows[0].SQL, "c2lua2NhbmFyeS1ib2R5", "INSERT INTO")
	hasAll(t, "error", rows[0].Error, sinkSecret, "duplicate key value", "violates constraint")
}

func TestTeamAndNodeSecurityEventsMaskRegisteredSecrets(t *testing.T) {
	secretmask.Add(sinkSecret)
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	err := db.RecordTeamEvent(ctx, TeamEvent{ID: "te_1", TeamID: "t1", ActorID: "u1", ActorName: "a@example.com",
		Action: "team.renamed", Subject: "renamed to " + sinkSecret + " team", Detail: "was " + sinkSecret, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	evs, err := db.ListTeamEvents(ctx, "t1", 10)
	if err != nil || len(evs) != 1 {
		t.Fatalf("team events = %v, %v", evs, err)
	}
	hasAll(t, "team subject", evs[0].Subject, sinkSecret, "renamed to ", " team")
	hasAll(t, "team detail", evs[0].Detail, sinkSecret, "was ")

	now := time.Now()
	ev := NodeSecurityEvent{NodeID: "n1", Kind: SecurityKindCertIssued, Outcome: SecurityOutcomeRejected, Reason: SecurityReasonEnrollRefused,
		SourceIP: "10.0.0.1", Detail: NodeSecurityDetail{Message: "refused token " + sinkSecret + " expired", Fingerprint: "ab12"}}
	pol := SecurityEventPolicy{Coalesce: true, Window: time.Minute, PerNodeCap: 50}
	// The second write takes the coalescing UPDATE path, which has its own detail bind.
	for range 2 {
		if err := db.AddNodeSecurityEvent(ctx, ev, pol, now); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.ListNodeSecurityEvents(ctx, "n1", NodeSecurityFilter{})
	if err != nil || len(got) != 1 || got[0].Count != 2 {
		t.Fatalf("security events = %+v, %v", got, err)
	}
	hasAll(t, "node message", got[0].Detail.Message, sinkSecret, "refused token ", " expired")
	if got[0].Detail.Fingerprint != "ab12" {
		t.Errorf("fingerprint = %q, want ab12", got[0].Detail.Fingerprint)
	}
}

// The hot sinks add no allocation when nothing matches.
func TestMaskJSONBytesNoHitDoesNotCopy(t *testing.T) {
	secretmask.Add(sinkSecret)
	in := []byte(`{"a":"plain text"}`)
	allocs := testing.AllocsPerRun(50, func() { _ = maskJSONBytes(in) })
	if allocs > 1 {
		t.Errorf("allocs per no-hit call = %v, want at most the string conversion", allocs)
	}
}

func TestDeployExitCaptureMasksRegisteredSecrets(t *testing.T) {
	secretmask.Add(sinkSecret)
	db := openTestDB(t)
	ctx := context.Background()
	in := []DeployExitLine{{Stream: "stderr", Message: "fatal: bad token " + sinkSecret + " rejected"}, {Stream: "stdout", Message: "clean line"}}
	err := db.RecordDeployExit(ctx, DeployExit{ServiceName: "web", AttemptID: "a1", Image: "app:v1", Message: "exited, last said " + sinkSecret, LogTail: in})
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.GetDeployExit(ctx, "web", "a1")
	if err != nil || got == nil || len(got.LogTail) != 2 {
		t.Fatalf("exit = %+v, %v", got, err)
	}
	hasAll(t, "message", got.Message, sinkSecret, "exited, last said ")
	hasAll(t, "tail", got.LogTail[0].Message, sinkSecret, "fatal: bad token ", " rejected")
	if got.LogTail[1].Message != "clean line" {
		t.Errorf("clean line changed: %q", got.LogTail[1].Message)
	}
	if !strings.Contains(in[0].Message, sinkSecret) {
		t.Error("caller's slice was mutated")
	}
}
