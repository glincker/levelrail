package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestStatusPageV2_GroupsBrandingAndPublicRoutes(t *testing.T) {
	rt, _, cookie := newStatusRouter(t, 100)

	for name, body := range map[string]string{
		"bad accent":  `{"enabled":true,"accent":"red"}`,
		"http logo":   `{"enabled":true,"logo_url":"http://cdn.example.org/l.png"}`,
		"bad website": `{"enabled":true,"website":"javascript:alert(1)"}`,
	} {
		if rec := apiDo(t, rt, cookie, http.MethodPut, "/api/v1/status-page", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", name, rec.Code)
		}
	}
	ok := `{"enabled":true,"title":"Acme","accent":"#ff6600","logo_url":"https://cdn.example.org/l.png","about":"hi","noindex":true}`
	if rec := apiDo(t, rt, cookie, http.MethodPut, "/api/v1/status-page", ok); rec.Code != http.StatusOK {
		t.Fatalf("branding save = %d %s", rec.Code, rec.Body.String())
	}
	// A client that omits branding must not reset it.
	if rec := apiDo(t, rt, cookie, http.MethodPut, "/api/v1/status-page", `{"enabled":true,"title":"Acme 2"}`); !strings.Contains(rec.Body.String(), `"accent":"#ff6600"`) {
		t.Errorf("omitted branding was reset: %s", rec.Body.String())
	}

	rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/groups", `{"name":"Core"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("group create = %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created.ID
	body := `{"kind":"domain","target":"example.com","display_name":"Site","group_id":"` + id + `","description":"Marketing site"}`
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/components", body); rec.Code != http.StatusCreated {
		t.Fatalf("component create = %d %s", rec.Code, rec.Body.String())
	}
	if rec := apiDo(t, rt, cookie, http.MethodPost, "/api/v1/status-page/components", strings.Replace(body, id, "sg_missing", 1)); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown group = %d, want 400", rec.Code)
	}

	page := publicGet(rt, "/public/status")
	if !strings.Contains(page.Body.String(), "Marketing site") || !strings.Contains(page.Header().Get("Content-Security-Policy"), "img-src https://cdn.example.org") {
		t.Errorf("page lacks description or logo CSP: %s", page.Header().Get("Content-Security-Policy"))
	}
	for path, ctype := range map[string]string{
		"/public/status.atom":         "application/atom+xml",
		"/public/status/history":      "text/html",
		"/public/status/badge.svg":    "image/svg+xml",
		"/public/status/history?tab=": "text/html",
	} {
		r := publicGet(rt, path)
		if r.Code != http.StatusOK || !strings.HasPrefix(r.Header().Get("Content-Type"), ctype) || r.Header().Get("ETag") == "" {
			t.Errorf("%s = %d %q", path, r.Code, r.Header().Get("Content-Type"))
		}
	}
	if r := publicGet(rt, "/public/status/incidents/nope"); r.Code != http.StatusNotFound {
		t.Errorf("unknown incident = %d, want 404", r.Code)
	}
	if r := publicGet(rt, "/public/status/badge.svg?component=nope"); r.Code != http.StatusNotFound {
		t.Errorf("unknown badge component = %d, want 404", r.Code)
	}
	if rec := apiDo(t, rt, cookie, http.MethodDelete, "/api/v1/status-page/groups/"+id, ""); rec.Code != http.StatusNoContent {
		t.Errorf("group delete = %d", rec.Code)
	}
}
