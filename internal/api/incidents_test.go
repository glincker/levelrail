package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/statuspage"
	"github.com/GLINCKER/levelrail/internal/store"
)

func seedInternalIncident(t *testing.T, db *store.DB, id string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Hour)
	in := store.InternalStatusIncident{
		StatusIncident:         store.StatusIncident{ID: id, Kind: statuspage.IncidentKind, Title: "INTTITLE db-prod-7", Status: "investigating", Impact: "major", StartsAt: now},
		StatusIncidentInternal: store.StatusIncidentInternal{Source: "monitor", SourceRef: "mon1", InternalState: store.IncidentStateTriggered},
	}
	if err := db.CreateInternalStatusIncident(ctx, in); err != nil {
		t.Fatal(err)
	}
	u := store.StatusIncidentUpdate{ID: "u" + id, IncidentID: id, Status: "investigating", Body: "INTBODY probe reason", CreatedAt: now}
	if err := db.AddInternalIncidentUpdate(ctx, u, store.UpdateVisibilityInternal, "system"); err != nil {
		t.Fatal(err)
	}
}

func incTokenGet(rt *Router, plain, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func incTokenPost(rt *Router, plain, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plain)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

// A read-only viewer token must not see internal incidents on the admin status page routes
// (which CLI and MCP reads use) nor on the incident routes.
func TestIncidents_ViewerTokenSeesNoInternal(t *testing.T) {
	rt, db, cookie := newStatusRouter(t, 100)
	rt.SetIncidents(db, 64)
	seedInternalIncident(t, db, "intid1")
	pub := store.StatusIncident{ID: "pubid1", Kind: statuspage.IncidentKind, Title: "Public outage", Status: "investigating", Impact: "minor", StartsAt: time.Now().UTC().Add(-time.Hour)}
	if err := db.SaveStatusIncident(context.Background(), pub); err != nil {
		t.Fatal(err)
	}
	if err := db.AddInternalIncidentUpdate(context.Background(), store.StatusIncidentUpdate{ID: "upriv", IncidentID: "pubid1", Status: "investigating", Body: "INTBODY2 private", CreatedAt: time.Now().UTC()}, store.UpdateVisibilityInternal, "op"); err != nil {
		t.Fatal(err)
	}
	viewer := seedMatrixToken(t, db, "viewer1", []string{AbilityRead})
	sensitive := seedMatrixToken(t, db, "sens1", []string{AbilityReadSensitive})

	list := incTokenGet(rt, viewer, "/api/v1/status-page/incidents")
	if list.Code != http.StatusOK {
		t.Fatalf("viewer list = %d", list.Code)
	}
	for _, n := range []string{"INTTITLE", "INTBODY", "intid1", "db-prod-7"} {
		if strings.Contains(list.Body.String(), n) {
			t.Errorf("viewer status-page list leaks %q", n)
		}
	}
	if !strings.Contains(list.Body.String(), "Public outage") {
		t.Error("viewer should still see the published incident")
	}
	if rec := incTokenPost(rt, viewer, "/api/v1/status-page/incidents/intid1/updates", `{"status":"resolved","body":"x"}`); rec.Code == http.StatusOK {
		t.Error("viewer must not post updates")
	}
	for _, p := range []string{"/api/v1/incidents", "/api/v1/incidents/intid1"} {
		if rec := incTokenGet(rt, viewer, p); rec.Code != http.StatusForbidden {
			t.Errorf("viewer %s = %d, want 403", p, rec.Code)
		}
	}
	if rec := incTokenGet(rt, sensitive, "/api/v1/incidents/intid1"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "INTBODY") {
		t.Errorf("read:sensitive get = %d %s", rec.Code, rec.Body.String())
	}
	// The admin write path cannot touch an unpublished incident.
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/incidents/intid1/updates", `{"status":"resolved","body":"x"}`); rec.Code != http.StatusNotFound {
		t.Errorf("public update on internal incident = %d, want 404", rec.Code)
	}
}

func TestIncidents_Lifecycle(t *testing.T) {
	rt, db, cookie := newStatusRouter(t, 100)
	if rec := apiDo(t, rt, cookie, http.MethodGet, "/api/v1/incidents", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("disabled list = %d, want 501", rec.Code)
	}
	rt.SetIncidents(db, 16)
	seedInternalIncident(t, db, "intid2")

	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/incidents/intid2/ack", ""); rec.Code != http.StatusOK {
		t.Fatalf("ack = %d %s", rec.Code, rec.Body.String())
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/incidents/intid2/ack", ""); rec.Code != http.StatusConflict {
		t.Errorf("second ack = %d, want 409", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/incidents/intid2/notes", `{"body":"`+strings.Repeat("a", 17)+`"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("oversize note = %d, want 400", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/incidents/intid2/notes", `{"body":"looking"}`); rec.Code != http.StatusCreated {
		t.Errorf("note = %d %s", rec.Code, rec.Body.String())
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/incidents/intid2/resolve", ""); rec.Code != http.StatusOK {
		t.Errorf("resolve = %d", rec.Code)
	}
	rec := apiDo(t, rt, cookie, http.MethodGet, "/api/v1/incidents/intid2", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"internal_state":"resolved"`) || !strings.Contains(rec.Body.String(), "looking") {
		t.Errorf("get = %d %s", rec.Code, rec.Body.String())
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/incidents", `{"source":"alert","rule_id":"nope"}`); rec.Code != http.StatusNotImplemented && rec.Code != http.StatusNotFound {
		t.Errorf("promote unknown rule = %d", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/incidents", `{"source":"monitor"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("promote bad source = %d, want 400", rec.Code)
	}
}
