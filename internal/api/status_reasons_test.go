package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
)

func TestStatusReasonsListsEveryCatalogEntry(t *testing.T) {
	rt, _, cookie := newSafetyRouter(t, stubResolver{})
	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/status-reasons", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got []deploysafety.ReasonInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if want := len(deploysafety.ReasonCatalog()); len(got) != want {
		t.Fatalf("got %d reasons, want %d", len(got), want)
	}
	for _, r := range got {
		if r.Code == "" || r.Meaning == "" || r.NextStep == "" {
			t.Errorf("incomplete entry %+v", r)
		}
	}
}

func TestStatusReasonsNeedsReadAbility(t *testing.T) {
	fixtures := loadRealDevFixtures(t)
	rt, _ := newDevFixturesTestRouter(t, fixtures)
	denied := serve(rt, bearerRequest(http.MethodGet, "/api/v1/status-reasons", "", fixturePlaintext(t, fixtures, "dev-read-sensitive")))
	if denied.Code != http.StatusForbidden {
		t.Errorf("token without read = %d, want 403", denied.Code)
	}
	ok := serve(rt, bearerRequest(http.MethodGet, "/api/v1/status-reasons", "", fixturePlaintext(t, fixtures, "dev-read")))
	if ok.Code != http.StatusOK {
		t.Errorf("read token = %d, want 200", ok.Code)
	}
}
