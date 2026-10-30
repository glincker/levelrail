package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMailStatusNotInstalled(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/mail/status", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got mailStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Installed || got.ContainerState != "none" || got.NextStep == "" {
		t.Fatalf("unexpected %+v", got)
	}
}

func TestMailNextStep(t *testing.T) {
	if s := mailNextStep(mailStatusResponse{Installed: true, ContainerState: "running", AdminReachable: true, TLS: "self-signed"}); s == "" {
		t.Fatal("empty")
	}
}
