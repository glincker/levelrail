package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

const importFixtureValue = "src-token-fixture"

func newImportSourceServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("source got non-GET %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("x-api-key") != importFixtureValue {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/project.all":
			_, _ = w.Write([]byte(`[{"projectId":"p1","name":"Shop","applications":[{"applicationId":"a1","name":"Storefront"}],"postgres":[{"postgresId":"d1","name":"shop-db","dockerImage":"postgres:15"}]}]`))
		case "/api/application.one":
			_, _ = w.Write([]byte(`{"applicationId":"a1","name":"Storefront","sourceType":"docker","dockerImage":"nginx:1.27","env":"MODE=prod\nAPI_KEY=abc123","replicas":2,"domains":[{"host":"shop.example.com","port":8080}],"mounts":[{"type":"volume","volumeName":"uploads","mountPath":"/data"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func importBody(url string) string {
	return `{"platform":"dokploy","url":"` + url + `","token":"` + importFixtureValue + `","allow_loopback":true}`
}

func TestPlatformImportDiscoverAndApply(t *testing.T) {
	t.Setenv(platformimport.AllowLoopbackEnv, "true")
	src := newImportSourceServer(t)
	setter := &fakeSecretSetter{}
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithSecretSetter(setter))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/imports/platform/discover", importBody(src.URL)))
	if rec.Code != http.StatusOK {
		t.Fatalf("discover status %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), importFixtureValue) || strings.Contains(rec.Body.String(), "abc123") {
		t.Fatalf("discover leaked a credential or secret value: %s", rec.Body.String())
	}
	var rep platformimport.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil || len(rep.Items) != 2 {
		t.Fatalf("report: %v %s", err, rec.Body.String())
	}
	if _, err := db.GetDesiredService(t.Context(), "storefront"); err == nil {
		t.Fatal("discover must not create anything")
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/imports/platform/apply", importBody(src.URL)))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply status %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), importFixtureValue) || strings.Contains(rec.Body.String(), "abc123") {
		t.Fatalf("apply leaked a credential or secret value: %s", rec.Body.String())
	}
	svc, err := db.GetDesiredService(t.Context(), "storefront")
	if err != nil {
		t.Fatalf("app not created: %v", err)
	}
	if svc.Image != "nginx:1.27" || svc.Port != 8080 || svc.Replicas != 2 || svc.Env["MODE"] != "prod" || svc.Labels[platformimport.LabelSourceID] != "dokploy:a1" {
		t.Errorf("app: %+v", svc)
	}
	if _, has := svc.Env["API_KEY"]; has {
		t.Error("secret-looking var must not be stored as plain env")
	}
	if len(setter.sets) != 1 || setter.sets[0].service != "storefront" || setter.sets[0].key != "API_KEY" || setter.sets[0].value != "abc123" {
		t.Errorf("secret sets: %+v", setter.sets)
	}
	if len(svc.Volumes) != 1 || svc.Volumes[0].Name != "app-storefront-uploads" {
		t.Errorf("volumes: %+v", svc.Volumes)
	}
	if _, err := db.GetDesiredDatabase(t.Context(), "shop-db"); err != nil {
		t.Errorf("database not created: %v", err)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/imports/platform/apply", importBody(src.URL)))
	var again platformimport.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &again); err != nil {
		t.Fatal(err)
	}
	if again.Counts[platformimport.StatusAlready] != 2 || len(setter.sets) != 1 {
		t.Errorf("re-run must skip existing: counts=%v sets=%d", again.Counts, len(setter.sets))
	}
}

func TestPlatformImportRejectsUnsafeRequests(t *testing.T) {
	src := newImportSourceServer(t)
	rt, db := newTestRouterWithBuilderAndSecrets(t, &fakeBuilder{}, nil, &fakeSecretSetter{})
	cookie := loginTestSession(t, rt, db)
	cases := []struct {
		name, body string
		want       int
	}{
		{"loopback opt-in without env", importBody(src.URL), http.StatusBadRequest},
		{"bad platform", `{"platform":"nope","url":"http://x","token":"t"}`, http.StatusBadRequest},
		{"missing token", `{"platform":"coolify","url":"http://x"}`, http.StatusBadRequest},
		{"bad collision", `{"platform":"coolify","url":"http://x","token":"t","collision":"replace"}`, http.StatusBadRequest},
		{"metadata address", `{"platform":"coolify","url":"http://169.254.169.254","token":"t","allow_private":false}`, http.StatusBadGateway},
		{"loopback blocked by default", `{"platform":"coolify","url":"` + src.URL + `","token":"t"}`, http.StatusBadGateway},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/imports/platform/discover", c.body))
		if rec.Code != c.want {
			t.Errorf("%s: status %d, want %d: %s", c.name, rec.Code, c.want, rec.Body.String())
		}
	}
}
