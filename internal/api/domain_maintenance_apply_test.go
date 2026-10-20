package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/maintenance"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestMaintenanceFailureError(t *testing.T) {
	f := &maintenanceFailure{Status: http.StatusBadRequest, Domain: "a.example.com", Stage: "validate", Err: errors.New("bad title")}
	got := f.Error()
	for _, want := range []string{"validate", "a.example.com", "bad title"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, want it to contain %q", got, want)
		}
	}
}

func TestEnableMaintenance_RejectsInvalidInputWithoutSavingAnything(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomains(t, db, "a.example.com", "b.example.com")
	cookie := loginTestSession(t, rt, db)

	tests := []struct {
		name string
		body string
	}{
		{"negative retry after", `{"retry_after_seconds": -5}`},
		{"end time in the past", `{"until": "2001-01-01T00:00:00Z"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/maintenance", tc.body))
			if rec.Code < 400 {
				t.Fatalf("status = %d, want an error; body = %s", rec.Code, rec.Body.String())
			}
			for _, d := range []string{"a.example.com", "b.example.com"} {
				on, err := db.GetDomainMaintenance(context.Background(), d)
				if err != nil {
					t.Fatalf("GetDomainMaintenance(%s): %v", d, err)
				}
				if on {
					t.Errorf("%s is in maintenance after a rejected request, want all or nothing", d)
				}
			}
		})
	}
}

func TestEnableThenClearMaintenance_AuditsBothEdges(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomains(t, db, "a.example.com")
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/maintenance", `{"title":"Upgrade"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("enable status = %d; body = %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/maintenance", ""))
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("clear status = %d; body = %s", rec.Code, rec.Body.String())
	}

	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatalf("ListAuditEntries: %v", err)
	}
	var started, ended bool
	for _, e := range entries {
		started = started || strings.Contains(e.Path+e.Ability, maintenance.AuditActionStarted)
		ended = ended || strings.Contains(e.Path+e.Ability, maintenance.AuditActionEnded)
	}
	if !started || !ended {
		t.Errorf("audit entries started=%t ended=%t, want both recorded (%d entries)", started, ended, len(entries))
	}
}

func TestClearMaintenance_IdempotentWhenNeverEnabled(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomains(t, db, "a.example.com")
	cookie := loginTestSession(t, rt, db)
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/maintenance", ""))
		if rec.Code >= 400 {
			t.Fatalf("clear #%d status = %d; body = %s", i+1, rec.Code, rec.Body.String())
		}
	}
}
