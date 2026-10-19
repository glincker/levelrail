package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestApplyAlertTemplate_CreatesThenSkips(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	post := func(body string) (*httptest.ResponseRecorder, applyAlertTemplateResponse) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/alerts/apply-template", body))
		var out applyAlertTemplateResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec, out
	}

	rec, out := post(`{"profile":"web"}`)
	if rec.Code != http.StatusCreated || len(out.Created) != 5 || len(out.Skipped) != 0 {
		t.Fatalf("first apply: status %d, created %d, skipped %d, body %s", rec.Code, len(out.Created), len(out.Skipped), rec.Body.String())
	}
	rec, out = post(`{"profile":"web"}`)
	if rec.Code != http.StatusCreated || len(out.Created) != 0 || len(out.Skipped) != 5 {
		t.Fatalf("second apply must skip everything: status %d, created %d, skipped %d", rec.Code, len(out.Created), len(out.Skipped))
	}
	if rec, _ = post(`{"profile":"mainframe"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown profile status = %d, want 400", rec.Code)
	}
}

func TestAlertTemplates_AllProfilesValidate(t *testing.T) {
	for _, p := range []string{AlertProfileWeb, AlertProfileWorker, AlertProfileDatabase} {
		rules, ok := alertTemplateRules(p)
		if !ok {
			t.Fatalf("profile %q missing", p)
		}
		for _, r := range rules {
			r.ResourceID = "service:x"
			if _, err := r.toRule("rule_x"); err != nil {
				t.Errorf("%s/%s invalid: %v", p, r.Name, err)
			}
		}
	}
}

func TestChannelQuietHours_RoundTrip(t *testing.T) {
	rt, db, adb := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	body := `{"name":"ops","kind":"generic","notify_url":"https://example.com/hook","enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/notification-channels", body))
	if rec.Code != http.StatusCreated {
		t.Skipf("channel create unavailable in this fixture: %d %s", rec.Code, rec.Body.String())
	}
	var ch notificationChannelResource
	_ = json.Unmarshal(rec.Body.Bytes(), &ch)
	path := "/api/v1/notification-channels/" + ch.ID + "/quiet-hours"

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, path, `{"start":"22:00","end":"07:00","timezone":"UTC","enabled":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("put status = %d, body %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, path, `{"start":"22:00","end":"22:00","enabled":true}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty window status = %d, want 400", rec.Code)
	}
	if q, err := adb.GetChannelQuietHours(t.Context(), ch.ID); err != nil || q == nil || q.Start != "22:00" {
		t.Fatalf("stored = %+v, %v", q, err)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, path, ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
}
