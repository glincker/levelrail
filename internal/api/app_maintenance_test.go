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

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/maintenance"
	"github.com/GLINCKER/levelrail/internal/store"
)

type failingNoise struct {
	AlertNoise
	failDomain string
}

func (f failingNoise) CreateSilence(ctx context.Context, s alerting.Silence) error {
	if s.CreatedBy == maintenance.SilenceCreatorPrefix+f.failDomain {
		return errors.New("disk full")
	}
	return f.AlertNoise.CreateSilence(ctx, s)
}

type failingSave struct {
	*store.DB
}

func (failingSave) SaveDomainMaintenanceAll(context.Context, []store.DomainMaintenance) error {
	return errors.New("write failed")
}

func twoDomainApp(t *testing.T, db *store.DB) {
	t.Helper()
	svc := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"a.example.com", "b.example.com"}}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatal(err)
	}
}

func activeMaintenanceSilences(t *testing.T, adb *alerting.DB) []alerting.Silence {
	t.Helper()
	list, err := adb.ListActiveSilences(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var out []alerting.Silence
	for _, s := range list {
		if _, ok := maintenance.SilenceDomain(s.CreatedBy); ok {
			out = append(out, s)
		}
	}
	return out
}

func decodeApp(t *testing.T, body []byte) appMaintenanceResource {
	t.Helper()
	var res appMaintenanceResource
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	return res
}

func TestAppMaintenance_AllDomainsSilenceAndClear(t *testing.T) {
	rt, db, adb, cookie := newNoiseRouter(t)
	rt.SetScheduledMaintenance(db)
	twoDomainApp(t, db)

	rec := apiDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/maintenance", `{"title":"Upgrade"}`)
	res := decodeApp(t, rec.Body.Bytes())
	if rec.Code != http.StatusOK || !res.All || res.Active != 2 || !res.Domains[0].AlertsSilenced || !res.Domains[0].Ongoing {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body.String())
	}
	if got := activeMaintenanceSilences(t, adb); len(got) != 2 {
		t.Fatalf("silences = %d, want 2", len(got))
	}

	// Enabling twice leaves one silence per domain.
	apiDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/maintenance", `{"title":"Upgrade"}`)
	if got := activeMaintenanceSilences(t, adb); len(got) != 2 {
		t.Fatalf("silences after second enable = %d, want 2", len(got))
	}

	rec = apiDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/maintenance", "")
	if res = decodeApp(t, rec.Body.Bytes()); rec.Code != http.StatusOK || res.Enabled {
		t.Fatalf("DELETE = %d %s", rec.Code, rec.Body.String())
	}
	if got := activeMaintenanceSilences(t, adb); len(got) != 0 {
		t.Fatalf("silences after clear = %d, want 0", len(got))
	}
	// Clearing again, with nothing enabled, is a quiet success.
	if rec = apiDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/maintenance", ""); rec.Code != http.StatusOK {
		t.Fatalf("second DELETE = %d", rec.Code)
	}
}

func TestAppMaintenance_OneDomainFailingChangesNothing(t *testing.T) {
	rt, db, adb, cookie := newNoiseRouter(t)
	rt.SetScheduledMaintenance(db)
	rt.alertNoise = failingNoise{AlertNoise: adb, failDomain: "b.example.com"}
	twoDomainApp(t, db)

	rec := apiDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/maintenance", `{}`)
	var fail maintenanceFailureResource
	_ = json.Unmarshal(rec.Body.Bytes(), &fail)
	if rec.Code != http.StatusInternalServerError || fail.Domain != "b.example.com" || fail.Stage != "silence" || !fail.RolledBack {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body.String())
	}
	state, _ := rt.appMaintenanceState(context.Background(), "web", []string{"a.example.com", "b.example.com"}, time.Now())
	if state.Active != 0 || len(activeMaintenanceSilences(t, adb)) != 0 {
		t.Fatalf("partial state: %+v silences=%d", state, len(activeMaintenanceSilences(t, adb)))
	}
	if ws, _ := db.ListMaintenanceWindows(context.Background(), time.Now().Add(-time.Hour)); len(ws) != 0 {
		t.Fatalf("status entries left behind: %+v", ws)
	}

	// Retry after the fault clears succeeds.
	rt.alertNoise = adb
	if rec = apiDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/maintenance", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("retry = %d %s", rec.Code, rec.Body.String())
	}
}

func TestAppMaintenance_RowWriteFailureLiftsSilences(t *testing.T) {
	rt, db, adb, cookie := newNoiseRouter(t)
	rt.SetScheduledMaintenance(db)
	rt.domainMaintenance = failingSave{db}
	twoDomainApp(t, db)

	rec := apiDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/domains/a.example.com/maintenance", `{}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("PUT = %d", rec.Code)
	}
	if n := len(activeMaintenanceSilences(t, adb)); n != 0 {
		t.Fatalf("silences left after failed save = %d", n)
	}
	if ws, _ := db.ListMaintenanceWindows(context.Background(), time.Now().Add(-time.Hour)); len(ws) != 0 {
		t.Fatalf("status entries left: %+v", ws)
	}
}

func TestMaintenanceSilence_HealedWhenRowMissing(t *testing.T) {
	rt, db, adb, cookie := newNoiseRouter(t)
	twoDomainApp(t, db)
	apiDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/domains/a.example.com/maintenance", `{}`)
	// Row removed behind the API's back, as after a crash between the two writes.
	if err := db.DeleteDomainMaintenance(context.Background(), "a.example.com"); err != nil {
		t.Fatal(err)
	}
	x := &maintenance.DomainExpirer{Store: db, Silencer: &maintenance.Silencer{Store: adb}, Logger: rt.logger}
	if err := x.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(activeMaintenanceSilences(t, adb)); n != 0 {
		t.Fatalf("orphan silence kept: %d", n)
	}
}

func TestMaintenanceSilence_ManualExpiryRefused(t *testing.T) {
	rt, db, adb, cookie := newNoiseRouter(t)
	twoDomainApp(t, db)
	apiDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/domains/a.example.com/maintenance", `{}`)
	sils := activeMaintenanceSilences(t, adb)
	if len(sils) != 1 {
		t.Fatalf("silences = %d", len(sils))
	}
	rec := apiDo(t, rt, cookie, http.MethodDelete, "/api/v1/alert-silences/"+sils[0].ID, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expire = %d, want 409", rec.Code)
	}
	rec = apiDo(t, rt, cookie, http.MethodGet, "/api/v1/alert-silences", "")
	if !strings.Contains(rec.Body.String(), `"source":"maintenance"`) {
		t.Fatalf("list lacks maintenance label: %s", rec.Body.String())
	}
}

func TestMaintenanceAudit_FieldMatrix(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	started := now.Add(-90 * time.Minute)
	until, autoOff := now.Add(-time.Minute), now.Add(-time.Minute)
	tests := []struct {
		name   string
		row    store.DomainMaintenance
		reason string
		wantBy string
	}{
		{"until", store.DomainMaintenance{Domain: "a.example.com", StartedAt: started, EndsAt: until}, maintenance.EndUntil, maintenance.EndUntil},
		{"max duration", store.DomainMaintenance{Domain: "a.example.com", StartedAt: started, AutoOffAt: autoOff}, maintenance.EndMaxDuration, maintenance.EndMaxDuration},
		{"manual", store.DomainMaintenance{Domain: "a.example.com", StartedAt: started}, maintenance.EndManual, ""},
		{"domain removed", store.DomainMaintenance{Domain: "a.example.com", StartedAt: started}, maintenance.EndDomainRemoved, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantBy != "" {
				if got := maintenance.EndReasonFor(tt.row, now); got != tt.wantBy {
					t.Fatalf("EndReasonFor = %s, want %s", got, tt.wantBy)
				}
			}
			actor := maintenance.Actor{Type: "session", ID: "u1", Name: "Ada"}
			e, err := maintenance.NewAuditEntry(maintenance.AuditActionEnded, maintenance.EndedDetail(tt.row, "web", tt.reason, actor, now), actor, now)
			if err != nil {
				t.Fatal(err)
			}
			var d maintenance.AuditDetail
			if err := json.Unmarshal([]byte(e.Detail), &d); err != nil {
				t.Fatal(err)
			}
			if d.EndReason != tt.reason || d.DurationSeconds != 5400 || d.StartedAt == "" || d.Actor != "Ada" || d.App != "web" || e.ActorID != "u1" {
				t.Fatalf("detail = %+v entry = %+v", d, e)
			}
		})
	}
}

func TestMaintenanceAudit_ManualSetAndClearCarryDetail(t *testing.T) {
	rt, db, _, cookie := newNoiseRouter(t)
	twoDomainApp(t, db)
	apiDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/domains/a.example.com/maintenance", `{}`)
	apiDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/domains/a.example.com/maintenance", "")
	// Clearing a domain that is not in maintenance writes no end row.
	apiDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/domains/b.example.com/maintenance", "")

	started, _ := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: maintenance.AuditActionStarted})
	ended, _ := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: maintenance.AuditActionEnded})
	if len(started) != 1 || len(ended) != 1 {
		t.Fatalf("started=%d ended=%d", len(started), len(ended))
	}
	var d maintenance.AuditDetail
	_ = json.Unmarshal([]byte(ended[0].Detail), &d)
	if d.EndReason != maintenance.EndManual || d.StartedAt == "" || d.Actor == "" {
		t.Fatalf("end detail = %+v", d)
	}
	if !strings.Contains(started[0].Detail, `"started_at"`) {
		t.Fatalf("start detail = %s", started[0].Detail)
	}
}

func TestMaintenanceAttention_ScopedToReadableApps(t *testing.T) {
	rt, db, _, cookie := newNoiseRouter(t)
	twoDomainApp(t, db)
	apiDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/maintenance", `{}`)
	later := time.Now().Add(48 * time.Hour)

	req := authedRequest(t, cookie, http.MethodGet, "/api/v1/attention/feed", "")
	if got := rt.maintenanceAttentionItems(req, []string{AbilityRoot, AbilityRead}, later); len(got) != 2 {
		t.Fatalf("root sees %d items, want 2", len(got))
	}
	if got := rt.maintenanceAttentionItems(req, nil, later); len(got) != 0 {
		t.Fatalf("caller without read sees %d items", len(got))
	}
}

func TestMaintenanceAttention_PerAppScopeHidesDeniedApps(t *testing.T) {
	rt, db, _, cookie := newNoiseRouter(t)
	ctx := context.Background()
	for _, svc := range []store.DesiredService{
		{Name: "web", Image: "img:1", Port: 80, Domains: []string{"w.example.com"}},
		{Name: "api", Image: "img:1", Port: 80, Domains: []string{"a.example.com"}},
	} {
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatal(err)
		}
	}
	apiDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/maintenance", `{}`)
	apiDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/api/maintenance", `{}`)
	tok := seedMatrixToken(t, db, "scoped-tok", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-api", "Deny", "*", "app:api", store.PrincipalTypeToken, "scoped-tok")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/attention/feed", nil)
	bearer(tok)(req)
	got := rt.maintenanceAttentionItems(req, []string{AbilityRead}, time.Now().Add(48*time.Hour))
	if len(got) != 1 || got[0].Params["app"] != "web" {
		t.Fatalf("scoped caller sees %+v, want only web", got)
	}
}
