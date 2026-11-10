package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Drafting a resolved incident fills the impact window and alert list; publishing locks every edit.
func TestIncidentPostmortem_DraftAndPublishLock(t *testing.T) {
	rt, db, cookie := newStatusRouter(t, 100)
	rt.SetIncidents(db, 200)
	seedInternalIncident(t, db, "pm1")
	ctx := context.Background()
	down := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	if err := db.AddStatusIncidentLink(ctx, "pm1", "mon1", down); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkStatusIncidentLinkUp(ctx, "pm1", "mon1", down.Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/incidents/pm1/postmortem"
	if rec := apiDo(t, rt, cookie, http.MethodPost, base+"/draft", ""); rec.Code != http.StatusConflict {
		t.Fatalf("draft before resolve = %d, want 409", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/incidents/pm1/resolve", ""); rec.Code != http.StatusOK {
		t.Fatalf("resolve = %d", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/incidents/nope/postmortem/draft", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown incident draft = %d, want 404", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, base+"/draft", ""); rec.Code != http.StatusOK {
		t.Fatalf("draft = %d %s", rec.Code, rec.Body.String())
	}
	get := func() postmortemResource {
		rec := apiDo(t, rt, cookie, http.MethodGet, base, "")
		var p postmortemResource
		if rec.Code != http.StatusOK {
			t.Fatalf("get = %d", rec.Code)
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := get()
	if !p.ImpactStart.Equal(down) || p.ImpactEnd == nil || !p.ImpactEnd.After(down) {
		t.Fatalf("impact window = %v to %v, want start %v and an end", p.ImpactStart, p.ImpactEnd, down)
	}
	if len(p.Alerts) != 1 || p.Alerts[0].Ref != "mon1" || p.Alerts[0].DownAt == nil || p.Alerts[0].UpAt == nil {
		t.Fatalf("alerts = %+v, want the one linked monitor outage", p.Alerts)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, base+"/publish", ""); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("publish empty = %d, want 422", rec.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPut, base, `{"summary":"`+strings.Repeat("x", 9000)+`"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("oversize field = %d, want 400", rec.Code)
	}
	for _, bad := range []string{`{"summary":"a\u001b[2Jb"}`, `{"root_cause":"x\u202ey"}`, `{"action_items":["ok","\u0085"]}`} {
		if rec := apiDo(t, rt, cookie, http.MethodPut, base, bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("control chars %s = %d, want 400", bad, rec.Code)
		}
	}
	// Two editors of different fields, then a re-draft: nothing written is lost.
	for _, edit := range []string{`{"summary":"<b>db</b> pool exhausted","action_items":["raise pool"," ",""]}`, `{"root_cause":"pool size\n\tsecond line"}`} {
		if rec := apiDo(t, rt, cookie, http.MethodPut, base, edit); rec.Code != http.StatusOK {
			t.Fatalf("edit = %d %s", rec.Code, rec.Body.String())
		}
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, base+"/draft", ""); rec.Code != http.StatusOK {
		t.Fatalf("re-draft = %d", rec.Code)
	}
	if p = get(); p.RootCause != "pool size\n\tsecond line" {
		t.Fatalf("root cause after re-draft = %q", p.RootCause)
	}
	if p = get(); p.Summary != "<b>db</b> pool exhausted" || len(p.ActionItems) != 1 || len(p.Alerts) != 1 {
		t.Fatalf("after edit = %+v", p)
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, base+"/publish", ""); rec.Code != http.StatusOK {
		t.Fatalf("publish = %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPut, base, `{"summary":"changed"}`},
		{http.MethodPost, base + "/draft", ""},
		{http.MethodPost, base + "/publish", ""},
	} {
		if rec := apiDo(t, rt, cookie, c.method, c.path, c.body); rec.Code != http.StatusConflict {
			t.Fatalf("%s %s after publish = %d, want 409", c.method, c.path, rec.Code)
		}
	}
	if rec := apiDo(t, rt, cookie, http.MethodDelete, "/api/v1/status-page/incidents/pm1", ""); rec.Code != http.StatusConflict {
		t.Fatalf("delete with published postmortem = %d, want 409", rec.Code)
	}
	if _, err := db.ExecContext(ctx, `UPDATE incident_postmortems SET summary = 'x' WHERE incident_id = 'pm1'`); err == nil {
		t.Fatal("direct update of a published postmortem succeeded")
	}
	if p = get(); p.State != "published" || p.Summary != "<b>db</b> pool exhausted" || p.PublishedAt == nil {
		t.Fatalf("published = %+v", p)
	}
}
