package store

import (
	"context"
	"testing"
	"time"
)

func seedBuiltinGate(t *testing.T, db *DB) User {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDomainAuthGate(ctx, DomainAuthGate{Domain: "app.example.com", Enabled: true, Provider: GateProviderBuiltin, CheckPath: "/"}); err != nil {
		t.Fatal(err)
	}
	u := User{ID: "user_gate", Email: "gate@example.com", DisplayName: "Gate", CreatedAt: time.Now()}
	if err := db.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestGateToken_SingleUse(t *testing.T) {
	db := openTestDB(t)
	u := seedBuiltinGate(t, db)
	ctx := context.Background()
	if err := db.CreateGateToken(ctx, GateToken{TokenHash: "h1", Domain: "app.example.com", UserID: u.ID, ParentSessionID: "s", StateHash: "st", ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	got, found, err := db.ConsumeGateToken(ctx, "h1")
	if err != nil || !found || got.Domain != "app.example.com" || got.UserID != u.ID || got.StateHash != "st" {
		t.Fatalf("first consume = %+v %v %v", got, found, err)
	}
	if _, found, err := db.ConsumeGateToken(ctx, "h1"); err != nil || found {
		t.Errorf("second consume found = %v, err %v", found, err)
	}
}

func TestGate_ReleasedWithGateAndUser(t *testing.T) {
	db := openTestDB(t)
	u := seedBuiltinGate(t, db)
	ctx := context.Background()
	if err := db.ReplaceGateRules(ctx, "app.example.com", []GateRule{{Kind: GateRuleUser, Subject: u.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateGateSession(ctx, GateSession{TokenHash: "s1", Domain: "app.example.com", UserID: u.ID, ParentSessionID: "p", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if rules, _ := db.ListGateRules(ctx, "app.example.com"); len(rules) != 0 {
		t.Errorf("rules after user delete = %v", rules)
	}
	if _, found, _ := db.GetGateSession(ctx, "s1"); found {
		t.Error("gate session survived user removal")
	}

	u2 := User{ID: "user_two", Email: "two@example.com", DisplayName: "Two", CreatedAt: time.Now()}
	if err := db.CreateUser(ctx, u2); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateGateSession(ctx, GateSession{TokenHash: "s2", Domain: "app.example.com", UserID: u2.ID, ParentSessionID: "p", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteDomainAuthGate(ctx, "app.example.com"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := db.GetGateSession(ctx, "s2"); found {
		t.Error("gate session survived gate removal")
	}
}

func TestRecordAuthGateEvent_Coalesces(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 11, 10, 0, 5, 0, time.UTC)
	for i, e := range []GateEvent{
		{At: at, Domain: "a.test", UserID: "u", Decision: GateDecisionAllowed, IP: "1.1.1.1"},
		{At: at.Add(20 * time.Second), Domain: "a.test", UserID: "u", Decision: GateDecisionAllowed, IP: "1.1.1.1", Count: 4},
		{At: at.Add(time.Minute), Domain: "a.test", UserID: "u", Decision: GateDecisionAllowed, IP: "1.1.1.1"},
		{At: at, Domain: "a.test", Decision: GateDecisionLoginRedirect, Reason: "no_session", IP: "2.2.2.2"},
	} {
		if err := db.RecordAuthGateEvent(ctx, e); err != nil {
			t.Fatalf("event %d: %v", i, err)
		}
	}
	events, err := db.ListAuthGateEvents(ctx, "a.test", at.Add(-time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("rows = %d (%+v), want 3", len(events), events)
	}
	counts := map[string]int{}
	for _, e := range events {
		counts[e.At.Format("15:04")+e.Decision] += e.Count
	}
	if counts["10:00allowed"] != 5 || counts["10:01allowed"] != 1 || counts["10:00login_redirect"] != 1 {
		t.Errorf("counts = %v", counts)
	}
	n, err := db.PruneGateData(ctx, at.Add(30*time.Second))
	if err != nil || n != 2 {
		t.Errorf("prune = %d, %v; want the two 10:00 rows older than the cutoff", n, err)
	}
}
