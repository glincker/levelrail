package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStackTemplates_HTTP(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	readerCookie := sessionCookieForTest(t, rt, reader.ID)
	do := func(c *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, c, method, path, body))
		return rec
	}

	rec := do(readerCookie, http.MethodGet, "/api/v1/stack-templates", "")
	var list struct {
		Templates []stackTemplateSummary `json:"templates"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil || len(list.Templates) != 5 {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}

	rec = do(readerCookie, http.MethodGet, "/api/v1/stack-templates/web-app-postgres", "")
	var d stackTemplateDetail
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &d) != nil {
		t.Fatalf("detail = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Join(d.SecretNames, ",") != "SESSION_SECRET" || !strings.Contains(strings.Join(d.ResourceKinds, ","), "Database") || len(d.Files) < 5 {
		t.Fatalf("detail = %+v", d.stackTemplateSummary)
	}
	if rec := do(readerCookie, http.MethodGet, "/api/v1/stack-templates/nope", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing = %d", rec.Code)
	}

	for _, tpl := range list.Templates {
		if rec := do(cookie, http.MethodPost, "/api/v1/stack-templates/"+tpl.ID+"/render", `{}`); rec.Code != http.StatusOK {
			t.Fatalf("render %s = %d %s", tpl.ID, rec.Code, rec.Body.String())
		}
	}

	path := "/api/v1/stack-templates/web-app-postgres/render"
	rec = do(cookie, http.MethodPost, path, `{"variables":{"name":"shop","domain":"shop.example.org"}}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "shop.example.org") || strings.Contains(rec.Body.String(), `"issues"`) {
		t.Fatalf("render = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(cookie, http.MethodPost, path, `{"variables":{"bogus":"x"}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad var = %d", rec.Code)
	}
	if rec := do(readerCookie, http.MethodPost, path, `{}`); rec.Code != http.StatusForbidden {
		t.Fatalf("reader render = %d", rec.Code)
	}
}
