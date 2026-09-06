package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/firewall"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeFirewallManager is a hand-written fake for FirewallManager,
// recording each call's want list so tests can assert the endpoint
// derived the right rules from seeded desired state.
type fakeFirewallManager struct {
	reportResult, syncResult   firewall.Result
	reportErr, syncErr         error
	gotReportWant, gotSyncWant []firewall.Rule
	syncCalls                  int
}

func (f *fakeFirewallManager) Report(_ context.Context, want []firewall.Rule) (firewall.Result, error) {
	f.gotReportWant = want
	return f.reportResult, f.reportErr
}

func (f *fakeFirewallManager) Sync(_ context.Context, want []firewall.Rule) (firewall.Result, error) {
	f.syncCalls++
	f.gotSyncWant = want
	return f.syncResult, f.syncErr
}

func newTestRouterWithFirewallManager(t *testing.T, m FirewallManager) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewRouter(discardLogger(), testBrand(), db, WithFirewallManager(m)), db
}

func TestHandleGetFirewallStatus_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithFirewallManager
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/firewall", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestFirewallStatusRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/firewall", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleGetFirewallStatus_DerivesWantFromDesiredState(t *testing.T) {
	fake := &fakeFirewallManager{reportResult: firewall.Result{Installed: true, Active: true, Managed: []firewall.RuleStatus{
		{Rule: firewall.Rule{Port: 8080, Owner: "app:web"}, Open: true},
	}}}
	rt, db := newTestRouterWithFirewallManager(t, fake)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	hostPort := 8080
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 3000, HostPort: &hostPort}); err != nil {
		t.Fatalf("seed service: %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "mydb", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	if _, err := db.SetDatabasePublicAccess(ctx, "mydb", true, 20005); err != nil {
		t.Fatalf("set public access: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/firewall", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if len(fake.gotReportWant) != 2 {
		t.Fatalf("Report() called with %d rules, want 2 (web's HostPort and mydb's public port): %+v", len(fake.gotReportWant), fake.gotReportWant)
	}
	byOwner := map[string]int{}
	for _, r := range fake.gotReportWant {
		byOwner[r.Owner] = r.Port
	}
	if byOwner["app:web"] != 8080 {
		t.Errorf("app:web port = %d, want 8080", byOwner["app:web"])
	}
	if byOwner["db:mydb"] != 20005 {
		t.Errorf("db:mydb port = %d, want 20005", byOwner["db:mydb"])
	}

	var got firewallStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Installed || !got.Active {
		t.Errorf("got = %+v, want Installed/Active true", got)
	}
	if len(got.Rules) != 1 || got.Rules[0].Port != 8080 || !got.Rules[0].Open {
		t.Errorf("Rules = %+v, want one open rule on 8080", got.Rules)
	}
}

func TestHandleGetFirewallStatus_NeverCallsSync(t *testing.T) {
	fake := &fakeFirewallManager{reportResult: firewall.Result{Installed: true, Active: true}}
	rt, db := newTestRouterWithFirewallManager(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/firewall", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if fake.syncCalls != 0 {
		t.Errorf("Sync() called %d times from a GET request, want 0", fake.syncCalls)
	}
}

func TestHandleGetFirewallStatus_ReportError(t *testing.T) {
	fake := &fakeFirewallManager{reportErr: errors.New("boom")}
	rt, db := newTestRouterWithFirewallManager(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/firewall", ""))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleSyncFirewall_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithFirewallManager
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/firewall/sync", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

// TestHandleSyncFirewall_PlainWriteToken_Forbidden proves POST
// /system/firewall/sync sits behind AbilityRoot, not AbilityWrite: it
// changes the actual host firewall fleet-wide, the same boundary
// TestHandleSystemPrune_PlainWriteToken_Forbidden already draws for
// POST /system/prune.
func TestHandleSyncFirewall_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouterWithFirewallManager(t, &fakeFirewallManager{})
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/firewall/sync", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach a fleet-wide firewall change", rec.Code, http.StatusForbidden)
	}
}

func TestHandleSyncFirewall_Success(t *testing.T) {
	fake := &fakeFirewallManager{syncResult: firewall.Result{
		Installed: true, Active: true,
		Managed: []firewall.RuleStatus{{Rule: firewall.Rule{Port: 8080, Owner: "app:web"}, Open: true}},
		Applied: 1,
	}}
	rt, db := newTestRouterWithFirewallManager(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/firewall/sync", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fake.syncCalls != 1 {
		t.Errorf("Sync() called %d times, want 1", fake.syncCalls)
	}

	var got firewallSyncResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Applied != 1 {
		t.Errorf("Applied = %d, want 1", got.Applied)
	}
	if len(got.Rules) != 1 || !got.Rules[0].Open {
		t.Errorf("Rules = %+v, want one open rule", got.Rules)
	}
}

func TestHandleSyncFirewall_SurfacesSyncError(t *testing.T) {
	fake := &fakeFirewallManager{syncErr: errors.New("boom")}
	rt, db := newTestRouterWithFirewallManager(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/firewall/sync", ""))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
