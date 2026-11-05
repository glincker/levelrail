package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
)

func TestMigrationDryRunEndpoint(t *testing.T) {
	rt, cookie, _ := schemaTestRouter(t, "v1.0.0", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/updates/migration-dry-run", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	res := decodeBody[selfupgrade.MigrationDryRunResult](t, rec.Body.Bytes())
	if !res.OK || len(res.Applied) != 0 {
		t.Fatalf("a current database has nothing pending: %+v", res)
	}
}

func TestMigrationDryRunRefusesSecondConcurrentRun(t *testing.T) {
	rt, cookie, _ := schemaTestRouter(t, "v1.0.0", nil)
	migrationDryRunBusy <- struct{}{}
	defer func() { <-migrationDryRunBusy }()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/updates/migration-dry-run", ""))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestMigrationDryRunRequiresAuth(t *testing.T) {
	rt, _, _ := schemaTestRouter(t, "v1.0.0", nil)
	assertRoutesRequireAuth(t, rt, []routeCase{{http.MethodPost, "/api/v1/updates/migration-dry-run"}})
}
