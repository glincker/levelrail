package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/store"
)

// deletingSetter also records deletions so rollback is observable.
type deletingSetter struct {
	*fakeSecretSetter
	deleted []fakeSecretSetterCall
}

func (d *deletingSetter) DeleteValue(_ context.Context, svc, key string) error {
	d.deleted = append(d.deleted, fakeSecretSetterCall{service: svc, key: key})
	return nil
}

func admissionRouter(t *testing.T) (*Router, *store.DB, *deletingSetter, *http.Cookie) {
	t.Helper()
	setter := &deletingSetter{fakeSecretSetter: &fakeSecretSetter{}}
	rt, db := newTestRouterWithBuilderAndSecrets(t, &fakeBuilder{tag: "img:sha"}, newFakeFetch("/tmp/checkout", nil), setter)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "teamdb", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	return rt, db, setter, loginTestSession(t, rt, db)
}

func deploySpecSecretBody(single, key string) string {
	body := `{"repo_url":"https://example.com/x.git","ref":"main",`
	if single != "" {
		body += `"single_service_name":"` + single + `",`
	}
	return body + `"services":{"` + key + `":{"build":{"type":"dockerfile"},"port":3000,"env":{"API_KEY":{"secret":true,"value":"sk-abc"}}}}}`
}

func TestDeploySpecRefusedNamesWriteNoSecret(t *testing.T) {
	cases := []struct {
		name, app, single, key string
	}{
		{"reserved single name", "myapp", "vault", "web"},
		{"database single name", "myapp", "teamdb", "web"},
		{"reserved derived name", "dns", "", "cloudflare"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt, _, setter, cookie := admissionRouter(t)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/"+c.app+"/deploy-spec", deploySpecSecretBody(c.single, c.key)))
			if rec.Code < 400 || rec.Code >= 500 {
				t.Fatalf("status = %d, want 4xx; body = %s", rec.Code, rec.Body.String())
			}
			if setter.calls != 0 || len(setter.sets) != 0 {
				t.Fatalf("a refused name must not write a secret, got %+v", setter.sets)
			}
		})
	}
}

func TestDeploySpecOwnRedeployStillWritesSecret(t *testing.T) {
	rt, db, setter, cookie := admissionRouter(t)
	seedApp(t, db, "myapp-web")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/deploy-spec", deploySpecSecretBody("", "web")))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if setter.calls != 1 || setter.lastService != "myapp-web" {
		t.Fatalf("own redeploy must write its secret, calls=%d service=%q", setter.calls, setter.lastService)
	}
}

func TestPlatformImportAppNamedLikeDatabaseWritesNoSecret(t *testing.T) {
	rt, _, setter, _ := admissionRouter(t)
	a := &importApplier{rt: rt, providerRefs: true}
	_, err := a.CreateApp(context.Background(), platformimport.AppPlan{
		Name: "teamdb", Image: "nginx:1", Port: 80, Replicas: 1,
		Secrets: map[string]string{"POSTGRES_PASSWORD": "stolen"}, Labels: map[string]string{},
	})
	if err == nil {
		t.Fatal("an app named like a database must be refused")
	}
	if setter.calls != 0 {
		t.Fatalf("refused import wrote %d secrets", setter.calls)
	}
}

func TestAdmitNewServiceNameReservedAndDatabase(t *testing.T) {
	rt, _, _, _ := admissionRouter(t)
	for _, n := range []string{"vault", "dns-cloudflare", "teamdb"} {
		if err := rt.AdmitNewServiceName(context.Background(), n); err == nil {
			t.Errorf("%q must be refused", n)
		}
	}
	if err := rt.AdmitNewServiceName(context.Background(), "fresh-app"); err != nil {
		t.Errorf("a free name must pass: %v", err)
	}
}
