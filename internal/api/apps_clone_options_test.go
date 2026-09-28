package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDeriveCloneDomains(t *testing.T) {
	tests := []struct {
		name string
		src  []string
		opts cloneDomainOpts
		want []string
	}{
		{"none drops all", []string{"web.example.com"}, cloneDomainOpts{mode: "none"}, nil},
		{"suffix on first label", []string{"web.example.com", "api.example.org"}, cloneDomainOpts{mode: "suffix", suffix: "stg"}, []string{"web-stg.example.com", "api-stg.example.org"}},
		{"wildcard and bare skipped", []string{"*.example.com", "localhost"}, cloneDomainOpts{mode: "suffix", suffix: "stg"}, []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveCloneDomains(tc.src, tc.opts)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func newCloneSecretsRouter(t *testing.T) (*Router, *store.DB, *secrets.Manager) {
	t.Helper()
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	manager := secrets.NewManager(db, mk)
	return NewRouter(discardLogger(), testBrand(), db, WithSecretSetter(manager)), db, manager
}

func TestCloneApp_CopySecretsRebindsToNewSlot(t *testing.T) {
	rt, db, manager := newCloneSecretsRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := t.Context()
	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "img:1", Port: 80, Domains: []string{"web.example.com"},
		SecretEnv: []store.SecretEnvRef{{Name: "API_KEY", Required: true}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetValue(ctx, "web", "API_KEY", "s3cret"); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone",
		`{"new_name":"web-stg","copy_secrets":true,"domains":"suffix","domain_suffix":"stg"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	got, err := manager.Resolve(ctx, "web-stg", "API_KEY")
	if err != nil || got != "s3cret" {
		t.Fatalf("clone secret = %q, %v; want re-encrypted copy", got, err)
	}
	if _, err := db.GetServiceDEK(ctx, "web-stg"); err != nil {
		t.Errorf("clone must own its DEK: %v", err)
	}
	clone, err := db.GetDesiredService(ctx, "web-stg")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(clone.Domains, []string{"web-stg.example.com"}) {
		t.Errorf("clone domains = %v", clone.Domains)
	}
	src, _ := db.GetDesiredService(ctx, "web")
	if !reflect.DeepEqual(src.Domains, []string{"web.example.com"}) {
		t.Errorf("source domains changed: %v", src.Domains)
	}
}

func TestCloneApp_FailureLeavesNoPartialClone(t *testing.T) {
	rt, db, manager := newCloneSecretsRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := t.Context()
	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "img:1", Port: 80, SecretEnv: []store.SecretEnvRef{{Name: "API_KEY"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetValue(ctx, "web", "API_KEY", "s3cret"); err != nil {
		t.Fatal(err)
	}

	badEnv := httptest.NewRecorder()
	rt.Handler().ServeHTTP(badEnv, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{"new_name":"web2","environment_id":"env_missing"}`))
	if badEnv.Code != http.StatusBadRequest {
		t.Fatalf("bad environment status = %d body=%s", badEnv.Code, badEnv.Body.String())
	}
	if _, err := db.GetDesiredService(ctx, "web2"); err == nil {
		t.Fatal("a rejected environment must not leave a clone behind")
	}

	if _, err := db.ExecContext(ctx, `UPDATE service_secrets SET wrapped_dek = 'broken' WHERE service_name = 'web'`); err != nil {
		t.Fatal(err)
	}
	failed := httptest.NewRecorder()
	rt.Handler().ServeHTTP(failed, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{"new_name":"web2","copy_secrets":true}`))
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("failed secret copy status = %d body=%s", failed.Code, failed.Body.String())
	}
	if _, err := db.GetDesiredService(ctx, "web2"); err == nil {
		t.Fatal("a failed secret copy must roll the clone back")
	}
}

func TestCloneApp_CopySecretsNeedsReadSensitive(t *testing.T) {
	rt, db, _ := newCloneSecretsRouter(t)
	bootstrapTestAdmin(t, db)
	if err := db.SaveDesiredService(t.Context(), store.DesiredService{Name: "web", Image: "img:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	user := storeUserWithAbilitiesForTest(t, db, "w@example.com", []string{AbilityRead, AbilityWrite})
	cookie := sessionCookieForTest(t, rt, user.ID)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{"new_name":"web2","copy_secrets":true}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if _, err := db.GetDesiredService(t.Context(), "web2"); err == nil {
		t.Error("clone must not be created when the secret copy is refused")
	}
}

func TestClonePreview(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(t.Context(), store.DesiredService{
		Name: "web", Image: "img:1", Port: 80, Domains: []string{"web.example.com"},
		SecretEnv: []store.SecretEnvRef{{Name: "K"}},
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/clone/preview", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var p clonePreviewResource
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.WillCopy) == 0 || len(p.WillNotCopy) == 0 || !reflect.DeepEqual(p.SecretNames, []string{"K"}) {
		t.Errorf("preview = %+v", p)
	}
}
