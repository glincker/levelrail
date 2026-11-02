package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func listAppNames(t *testing.T, rt *Router, tok, query string) ([]string, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps?"+query, nil)
	bearer(tok)(req)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET apps?%s = %d %s", query, rec.Code, rec.Body.String())
	}
	var apps []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &apps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	names := make([]string, len(apps))
	for i, a := range apps {
		names[i] = a.Name
	}
	return names, rec.Header().Get("X-Total-Count")
}

func TestAppsList_QueryIsRankedTypoTolerantAndAuthorized(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	seedSearchApps(t, db, "billing-api", "api", "api-gateway", "rapid-worker", "web", "secret-api")
	root := seedMatrixToken(t, db, "root", []string{AbilityRoot})
	reader := seedMatrixToken(t, db, "reader", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-secret", "Deny", "*", "app:secret-api", store.PrincipalTypeToken, "reader")

	names, total := listAppNames(t, rt, root, "q=api")
	if len(names) == 0 || names[0] != "api" || total != "5" {
		t.Fatalf("q=api = %v total %s, want exact name first and 5 matches", names, total)
	}

	names, _ = listAppNames(t, rt, root, "q="+url.QueryEscape("rapd"))
	if len(names) == 0 || names[0] != "rapid-worker" {
		t.Fatalf("typo q=rapd = %v, want rapid-worker", names)
	}

	names, total = listAppNames(t, rt, reader, "q=api")
	if strings.Contains(strings.Join(names, ","), "secret-api") || total != "4" {
		t.Fatalf("reader q=api = %v total %s, must hide the denied app", names, total)
	}

	names, _ = listAppNames(t, rt, root, "q=api&limit=1&offset=1")
	if len(names) != 1 || names[0] == "api" {
		t.Fatalf("paging a ranked list = %v, want second hit only", names)
	}

	names, _ = listAppNames(t, rt, root, "q=img")
	if len(names) != 6 {
		t.Fatalf("image substring fallback = %v, want all 6 apps", names)
	}
}
