package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/mail/demo"
	"github.com/GLINCKER/levelrail/internal/mailrecords"
	"github.com/GLINCKER/levelrail/internal/store"
)

type nopAudit struct{ AuditStore }

func (nopAudit) SaveAuditEntry(context.Context, store.AuditEntry) error { return nil }

func mailDemoRouter(t *testing.T) *Router {
	t.Helper()
	var buf bytes.Buffer
	rt := &Router{logger: slog.New(slog.NewTextHandler(&buf, nil))}
	rt.auditLog = nopAudit{}
	rt.mail.demo = &mailDemo{engine: demo.NewEngine(), zone: demo.NewZone()}
	return rt
}

func TestMailDemoOffUsesRealImplementations(t *testing.T) {
	rt := &Router{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if rt.mail.demo != nil {
		t.Fatal("demo must be nil by default")
	}
	if _, ok := rt.mailResolver().(*demo.Zone); ok {
		t.Fatal("resolver must be the real one with demo off")
	}
	w := httptest.NewRecorder()
	if rt.mailDemoStatus(w, httptest.NewRequest(http.MethodGet, "/", nil)) || rt.mailDemoPorts(w) || rt.mailDemoPortsOpen(w) {
		t.Fatal("demo seams must decline with demo off")
	}
	s := mailrecords.Spec{Domain: "example.com"}
	rt.mailDemoFillSpec(&s)
	if s.IPv4 != "" || s.DKIMPublicKey != "" {
		t.Fatal("demo must not fill the spec with demo off")
	}
}

func TestMailDemoRequiresDevMode(t *testing.T) {
	t.Setenv(demo.EnableEnv, "1")
	var buf bytes.Buffer
	rt := &Router{logger: slog.New(slog.NewTextHandler(&buf, nil))}
	rt.initMailDemo()
	if devModeEnabled() {
		t.Skip("dev mode is on in this environment")
	}
	if rt.mail.demo != nil {
		t.Fatal("demo enabled without APP_DEV_MODE")
	}
	if strings.Count(buf.String(), "APP_MAIL_DEMO is ignored") != 1 {
		t.Fatalf("want exactly one warning, got %q", buf.String())
	}
}

func TestMailDemoEnabledGate(t *testing.T) {
	env := map[string]string{demo.EnableEnv: "1"}
	get := func(k string) string { return env[k] }
	if on, ign := demo.Enabled(false, get); on || !ign {
		t.Fatalf("without dev mode: on=%v ignored=%v", on, ign)
	}
	if on, ign := demo.Enabled(true, get); !on || ign {
		t.Fatalf("with dev mode: on=%v ignored=%v", on, ign)
	}
	if on, _ := demo.Enabled(true, func(string) string { return "" }); on {
		t.Fatal("dev mode alone must not enable demo")
	}
}

func TestMailDemoSecretsAreObviouslyFake(t *testing.T) {
	if !strings.Contains(demo.FakeMarker, "not-a-real") {
		t.Fatal("demo password must be obviously fake")
	}
	if demo.MailIPv4 != "203.0.113.10" {
		t.Fatal("demo IP must stay in the documentation range")
	}
}

func TestMailDemoPlanApplyCheckTurnsGreen(t *testing.T) {
	rt := mailDemoRouter(t)
	plan := func() mailrecords.PlanResult {
		w := httptest.NewRecorder()
		rt.handleMailDNSPlan(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"domain":"demo.example"}`)))
		if w.Code != http.StatusOK {
			t.Fatalf("plan %d %s", w.Code, w.Body)
		}
		var res mailrecords.PlanResult
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		return res
	}
	check := func() mailrecords.Report {
		w := httptest.NewRecorder()
		rt.handleMailDNSCheck(w, httptest.NewRequest(http.MethodGet, "/?domain=demo.example", nil))
		var rep mailrecords.Report
		if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
			t.Fatal(err)
		}
		return rep
	}
	before := check()
	failing := map[string]bool{}
	for _, it := range before.Items {
		if it.Status == mailrecords.StatusFail {
			failing[it.ID] = true
		}
	}
	if !failing["spf"] || !failing["dmarc"] {
		t.Fatalf("expected SPF and DMARC failures before apply, got %+v", before.Items)
	}
	p := plan()
	body, _ := json.Marshal(map[string]any{"domain": "demo.example", "confirm": true, "plan_hash": p.Hash})
	w := httptest.NewRecorder()
	rt.handleMailDNSApply(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("apply %d %s", w.Code, w.Body)
	}
	for _, it := range check().Items {
		if it.Status == mailrecords.StatusFail {
			t.Errorf("still failing after apply: %+v", it)
		}
	}
	d := rt.mail.demo
	d.zone.Reset()
	for _, it := range check().Items {
		if it.ID == "spf" && it.Status != mailrecords.StatusFail {
			t.Error("reset must bring the SPF failure back")
		}
	}
}

func TestMailDemoEngineLifecycle(t *testing.T) {
	e := demo.NewEngine()
	ctx := context.Background()
	st, _ := e.QueueStats(ctx)
	if st.Queued < 3 || st.Failing < 1 {
		t.Fatalf("fixtures must include a failing message: %+v", st)
	}
	if n, _ := e.HoldMessages(ctx, []int64{101}, true); n != 1 {
		t.Fatal("hold")
	}
	_ = e.ReleaseQueue(ctx)
	if err := e.SetLoginDisabled(ctx, "alice", "demo"); err != nil {
		t.Fatal(err)
	}
	if d, _ := e.AccountDetail(ctx, "alice"); !d.LoginDisabled {
		t.Fatal("suspend")
	}
	if err := e.RemoveAccount(ctx, "bob"); err != nil {
		t.Fatal(err)
	}
	if err := e.RemoveAccount(ctx, "bob"); err == nil {
		t.Fatal("second delete must fail")
	}
}

func TestMailDemoRootClaimOnlyForDevOwner(t *testing.T) {
	res := []string{"mail:domain:demo.example"}
	root := []string{AbilityRoot}
	if got := mailDecide(nil, root, nil, store.DefaultTeamID, AbilityRoot, res...); got != http.StatusForbidden {
		t.Fatalf("precondition: capped root claim should be refused, got %d", got)
	}
	if got := mailDecide(nil, root, nil, store.DefaultTeamID, AbilityWriteSensitive, res...); got != 0 {
		t.Fatalf("dev owner creating a mailbox: got %d", got)
	}
	demoRT := mailDemoRouter(t)
	if !demoRT.mailDemoRootClaim(nil, root, AbilityRoot) {
		t.Fatal("demo must let the dev owner claim a domain")
	}
	member := &teamScope{roles: map[string]string{store.DefaultTeamID: store.TeamRoleDeveloper}}
	abilities := []string{AbilityRead, AbilityWriteSensitive}
	if demoRT.mailDemoRootClaim(member, abilities, AbilityRoot) {
		t.Fatal("a team member must still be refused the claim")
	}
	if got := mailDecide(member, abilities, nil, store.DefaultTeamID, AbilityRoot, res...); got != http.StatusForbidden {
		t.Fatalf("non-owner claim: got %d, want 403", got)
	}
	off := &Router{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if off.mailDemoRootClaim(nil, root, AbilityRoot) {
		t.Fatal("production behavior must not change with demo off")
	}
}
