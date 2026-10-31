package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/statuspage"
	"github.com/GLINCKER/levelrail/internal/store"
)

// An unpublished internal incident and its internal updates must never reach a public surface.
func TestStatusPage_InternalIncidentNeverPublic(t *testing.T) {
	rt, db, cookie := newStatusRouter(t, 100)
	ctx := context.Background()
	if rec := apiDo(t, rt, cookie, http.MethodPut, "/api/v1/status-page", `{"enabled":true,"title":"Acme"}`); rec.Code != http.StatusOK {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body.String())
	}
	now := time.Now().UTC().Add(-time.Hour)
	secret := store.InternalStatusIncident{
		StatusIncident:         store.StatusIncident{ID: "leakid9", Kind: statuspage.IncidentKind, Title: "LEAKTITLE db-prod-7 timeouts", Status: "investigating", Impact: "major", StartsAt: now},
		StatusIncidentInternal: store.StatusIncidentInternal{Source: "monitor", SourceRef: "mon1", InternalState: store.IncidentStateTriggered},
	}
	if err := db.CreateInternalStatusIncident(ctx, secret); err != nil {
		t.Fatal(err)
	}
	if err := db.AddInternalIncidentUpdate(ctx, store.StatusIncidentUpdate{ID: "leakup9", IncidentID: "leakid9", Status: "investigating", Body: "LEAKBODY probe stack trace", CreatedAt: now}, store.UpdateVisibilityInternal, "system"); err != nil {
		t.Fatal(err)
	}
	// An internal-visibility update on a published incident must stay hidden too.
	pub := store.StatusIncident{ID: "pubid1", Kind: statuspage.IncidentKind, Title: "Public outage", Status: "investigating", Impact: "minor", StartsAt: now}
	if err := db.SaveStatusIncident(ctx, pub); err != nil {
		t.Fatal(err)
	}
	if err := db.AddInternalIncidentUpdate(ctx, store.StatusIncidentUpdate{ID: "leakup8", IncidentID: "pubid1", Status: "investigating", Body: "LEAKBODY2 private note", CreatedAt: now}, store.UpdateVisibilityInternal, "op"); err != nil {
		t.Fatal(err)
	}
	rt.statusView.Invalidate()

	needles := []string{"LEAKTITLE", "LEAKBODY", "leakid9", "leakup9", "leakup8", "db-prod-7"}
	for _, path := range []string{"/public/status", "/public/status.atom", "/public/status/history", "/public/status/badge.svg", "/public/status/incidents/leakid9"} {
		body := publicGet(rt, path).Body.String()
		for _, n := range needles {
			if strings.Contains(body, n) {
				t.Errorf("%s leaks %q", path, n)
			}
		}
	}
	if !strings.Contains(publicGet(rt, "/public/status").Body.String(), "Public outage") {
		t.Error("published incident missing from the page")
	}

	rec := serveMobile(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/mobile/overview", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("overview = %d", rec.Code)
	}
	for _, n := range needles {
		if strings.Contains(rec.Body.String(), n) {
			t.Errorf("mobile overview leaks %q", n)
		}
	}
	if !strings.Contains(rec.Body.String(), `"open_incidents":1`) {
		t.Errorf("open_incidents should count only the published incident: %s", rec.Body.String())
	}
}
