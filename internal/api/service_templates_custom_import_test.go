package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

func TestCustomTemplateImport(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	const body = `{"id":"my-web","name":"My web","compose_yaml":"services:\n  web:\n    image: nginx:1\n"}`
	post := func(b string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/templates/custom", b))
		return rec
	}

	rec := post(body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "nginx") {
		t.Fatal("response echoes the compose body")
	}
	if rec := post(body); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d, want 409", rec.Code)
	}
	if rec := post(`{"id":"bad-one","name":"x","compose_yaml":"not: [compose"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid = %d, want 422", rec.Code)
	}
	if len(catalog.Templates) == 0 {
		t.Skip("empty catalog")
	}
	clash := `{"id":"` + catalog.Templates[0].ID + `","name":"x","compose_yaml":"services:\n  a:\n    image: x\n"}`
	if rec := post(clash); rec.Code != http.StatusConflict {
		t.Fatalf("built-in clash = %d, want 409", rec.Code)
	}
}
