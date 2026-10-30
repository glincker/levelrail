package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

func TestMailTemplateGatedByExperimentalFlag(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	t.Cleanup(func() { experimental.Set(experimental.All()...) })

	listed := func() bool {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/service-templates", ""))
		var items []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		for _, it := range items {
			if it.ID == "mail-mox" {
				return true
			}
		}
		return false
	}
	status := func(method, path string) int {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, "{}"))
		return rec.Code
	}

	experimental.Set()
	if listed() {
		t.Error("flag off: mail-mox must not be listed")
	}
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/service-templates/mail-mox"},
		{http.MethodPost, "/api/v1/service-templates/mail-mox/deploy"},
		{http.MethodPost, "/api/v1/service-templates/mail-mox/install"},
	} {
		if got := status(c.method, c.path); got != http.StatusNotFound {
			t.Errorf("flag off: %s %s = %d, want 404", c.method, c.path, got)
		}
	}

	experimental.Set(experimental.Mail)
	if !listed() {
		t.Error("flag on: mail-mox must be listed")
	}
	if got := status(http.MethodGet, "/api/v1/service-templates/mail-mox"); got != http.StatusOK {
		t.Errorf("flag on: get = %d, want 200", got)
	}
	if got := status(http.MethodPost, "/api/v1/service-templates/mail-mox/deploy"); got == http.StatusNotFound {
		t.Errorf("flag on: deploy must not be 404")
	}
}
