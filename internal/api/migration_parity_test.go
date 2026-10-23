package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/migrateverify"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

func verifyBody(url, extra string) string {
	return `{"platform":"dokploy","url":"` + url + `","token":"` + importFixtureValue + `","allow_loopback":true` + extra + `}`
}

func TestMigrationVerifyEndToEnd(t *testing.T) {
	t.Setenv(platformimport.AllowLoopbackEnv, "true")
	t.Setenv(migrateverify.EnvAllowPrivateProbe, "true")
	src := newImportSourceServer(t)
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithSecretSetter(&fakeSecretSetter{}))
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	if rec := do(http.MethodGet, "/api/v1/migrations/verify/latest", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("latest before any run: status %d", rec.Code)
	}

	rec := do(http.MethodPost, "/api/v1/migrations/verify", verifyBody(src.URL, `,"skip_probes":true`))
	if rec.Code != http.StatusOK {
		t.Fatalf("verify status %d: %s", rec.Code, rec.Body.String())
	}
	var before parityVerifyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &before); err != nil {
		t.Fatal(err)
	}
	if before.Verdict != migrateverify.VerdictFail || before.Counts.Blocking == 0 || !before.CanRecheck {
		t.Fatalf("an empty target must fail and allow a re-check: %+v", before)
	}
	for _, leak := range []string{importFixtureValue, "abc123"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("report leaked %q: %s", leak, rec.Body.String())
		}
	}

	if rec := do(http.MethodPost, "/api/v1/imports/platform/apply", importBody(src.URL)); rec.Code != http.StatusOK {
		t.Fatalf("apply status %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(http.MethodPost, "/api/v1/migrations/verify", `{"recheck":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-check status %d: %s", rec.Code, rec.Body.String())
	}
	var after parityVerifyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if after.Counts.Blocking != 0 {
		t.Fatalf("after import nothing should block: %+v", after.Differences)
	}

	rec = do(http.MethodGet, "/api/v1/migrations/verify/latest", "")
	var latest parityVerifyResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &latest) != nil || latest.Verdict != after.Verdict || !latest.GeneratedAt.Equal(after.GeneratedAt) {
		t.Fatalf("latest does not match the last run: %d %s", rec.Code, rec.Body.String())
	}
}

func TestMigrationVerifyProbesBothSides(t *testing.T) {
	t.Setenv(platformimport.AllowLoopbackEnv, "true")
	t.Setenv(migrateverify.EnvAllowPrivateProbe, "true")
	src := newImportSourceServer(t)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer site.Close()
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithSecretSetter(&fakeSecretSetter{}))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/imports/platform/apply", importBody(src.URL)))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply status %d: %s", rec.Code, rec.Body.String())
	}
	addr := site.Listener.Addr().String()
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/migrations/verify",
		verifyBody(src.URL, `,"allow_private_probes":true,"source_ip":"`+addr+`","target_ip":"`+addr+`","probe_timeout_seconds":3,"probe_concurrency":1`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("verify status %d: %s", rec.Code, rec.Body.String())
	}
	var rep parityVerifyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Probes) != 1 || rep.Probes[0].Source.Status != http.StatusOK || rep.Probes[0].Target.Status != http.StatusOK {
		t.Fatalf("want one domain probed on both sides: %+v", rep.Probes)
	}
}

func TestMigrationVerifyRefusesPrivateProbesWithoutOperatorOptIn(t *testing.T) {
	t.Setenv(platformimport.AllowLoopbackEnv, "true")
	t.Setenv(migrateverify.EnvAllowPrivateProbe, "")
	src := newImportSourceServer(t)
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/migrations/verify", verifyBody(src.URL, `,"allow_private_probes":true`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/migrations/verify", verifyBody(src.URL, `,"target_ip":"not-an-ip"`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a bad target_ip: status %d, want 400", rec.Code)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/migrations/verify", `{"recheck":true}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("re-check without a held credential: status %d, want 409", rec.Code)
	}
}
