package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

var publicSurfaces = []string{"/public/status", "/public/status.atom", "/public/status/history", "/public/status/incidents/pubflow1"}

func surfacesContain(rt *Router, needle string) []string {
	var hits []string
	for _, p := range publicSurfaces {
		if strings.Contains(publicGet(rt, p).Body.String(), needle) {
			hits = append(hits, p)
		}
	}
	return hits
}

// A published incident shows on the public surfaces only after publish, and internal entries never do.
func TestIncidents_PublishFlow(t *testing.T) {
	rt, db, cookie := newStatusRouter(t, 100)
	if rec := apiDo(t, rt, cookie, http.MethodPut, "/api/v1/status-page", `{"enabled":true,"title":"Acme"}`); rec.Code != http.StatusOK {
		t.Fatalf("enable = %d", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/incidents/pubflow1/publish", `{}`); rec.Code != http.StatusNotImplemented {
		t.Fatalf("disabled publish = %d, want 501", rec.Code)
	}
	rt.SetIncidents(db, 200)
	seedInternalIncident(t, db, "pubflow1")
	get := func() incidentResource {
		rec := apiDo(t, rt, cookie, http.MethodGet, "/api/v1/incidents/pubflow1", "")
		var res incidentResource
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		return res
	}
	before := get()
	stamp := `{"updated_at":"` + before.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00") + `"}`

	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/incidents/pubflow1/publish", stamp); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("publish with empty draft = %d, want 422", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPut, "/api/v1/incidents/pubflow1/draft", `{"title":"PUBTITLE Elevated errors","status":"bogus","body":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad status draft = %d, want 400", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPut, "/api/v1/incidents/pubflow1/draft", `{"title":"PUBTITLE Elevated errors","status":"identified","body":"PUBBODY we found the cause"}`); rec.Code != http.StatusOK {
		t.Fatalf("save draft = %d %s", rec.Code, rec.Body.String())
	}
	for _, n := range []string{"PUBTITLE", "PUBBODY", "INTBODY"} {
		if hits := surfacesContain(rt, n); len(hits) > 0 {
			t.Errorf("draft leaked %q on %v", n, hits)
		}
	}
	// The earlier save moved updated_at, so the old stamp is stale.
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/incidents/pubflow1/publish", stamp); rec.Code != http.StatusConflict {
		t.Fatalf("stale publish = %d, want 409", rec.Code)
	}
	fresh := get()
	if fresh.Draft == nil || fresh.Draft.Title != "PUBTITLE Elevated errors" {
		t.Fatalf("draft not returned to operators: %+v", fresh.Draft)
	}
	stamp = `{"updated_at":"` + fresh.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00") + `"}`
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/incidents/pubflow1/publish", stamp); rec.Code != http.StatusOK {
		t.Fatalf("publish = %d %s", rec.Code, rec.Body.String())
	}
	rt.statusView.Invalidate()

	if len(surfacesContain(rt, "PUBTITLE")) == 0 || len(surfacesContain(rt, "PUBTITLE")) < 3 {
		t.Errorf("published title should be on the page, atom, history and incident page, got %v", surfacesContain(rt, "PUBTITLE"))
	}
	if hits := surfacesContain(rt, "PUBBODY"); len(hits) == 0 {
		t.Error("published body missing from every surface")
	}
	for _, n := range []string{"INTTITLE", "INTBODY", "db-prod-7", "Public draft saved", "Published to the public status page"} {
		if hits := surfacesContain(rt, n); len(hits) > 0 {
			t.Errorf("internal text %q reached %v", n, hits)
		}
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/incidents/pubflow1/publish", `{"updated_at":"2030-01-01T00:00:00Z"}`); rec.Code != http.StatusConflict {
		t.Errorf("second publish = %d, want 409", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPut, "/api/v1/incidents/pubflow1/draft", `{"title":"t","body":"b"}`); rec.Code != http.StatusConflict {
		t.Errorf("draft after publish = %d, want 409", rec.Code)
	}
	// Later public updates use the existing route.
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/incidents/pubflow1/updates", `{"status":"resolved","body":"LATERUPDATE fixed"}`); rec.Code != http.StatusOK {
		t.Fatalf("public update = %d %s", rec.Code, rec.Body.String())
	}
	rt.statusView.Invalidate()
	if len(surfacesContain(rt, "LATERUPDATE")) == 0 {
		t.Error("public update missing after publish")
	}

	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Method: http.MethodPost})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if strings.HasSuffix(e.Path, "/incidents/pubflow1/publish") && e.StatusCode == http.StatusOK && e.ActorID != "" {
			found = true
		}
		if strings.Contains(e.Detail, "PUBBODY") {
			t.Error("audit row carries the draft body")
		}
	}
	if !found {
		t.Error("publish was not audited with the incident id and actor")
	}
}
