package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func seedDualCopyApp(t *testing.T, db *store.DB, env map[string]string, secretNames ...string) {
	t.Helper()
	svc := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Env: env, SecretEnv: store.SecretEnvRefsFromNames(secretNames)}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestDualCopyWriterPaths(t *testing.T) {
	tests := []struct {
		name       string
		setterErr  error
		locked     bool
		method     string
		path, body string
		env        map[string]string
		secrets    []string
		wantStatus int
		wantPlain  bool
		wantSecret bool
		wantBody   string
	}{
		{name: "secrets set replaces plain copy", method: http.MethodPut, path: "/api/v1/apps/web/secrets/K", body: `{"value":"new"}`,
			env: map[string]string{"K": "old"}, wantStatus: http.StatusOK, wantPlain: false, wantSecret: true, wantBody: `"replaced_plain_env":true`},
		{name: "secrets set without plain copy stays 204", method: http.MethodPut, path: "/api/v1/apps/web/secrets/K", body: `{"value":"new"}`,
			env: map[string]string{"OTHER": "x"}, wantStatus: http.StatusNoContent, wantSecret: true},
		{name: "secret store failure keeps the plain value", setterErr: errors.New("boom"), method: http.MethodPut, path: "/api/v1/apps/web/secrets/K", body: `{"value":"new"}`,
			env: map[string]string{"K": "old"}, wantStatus: http.StatusInternalServerError, wantPlain: true, wantSecret: false},
		{name: "locked secret keeps the plain value", locked: true, method: http.MethodPut, path: "/api/v1/apps/web/secrets/K", body: `{"value":"new"}`,
			env: map[string]string{"K": "old"}, wantStatus: http.StatusConflict, wantPlain: true, wantSecret: false},
		{name: "env set on a secret key is refused with a pointer", method: http.MethodPut, path: "/api/v1/apps/web/env", body: `{"set":{"K":"v"}}`,
			secrets: []string{"K"}, wantStatus: http.StatusConflict, wantSecret: true, wantBody: "apps secrets set"},
		{name: "update adding a key to both is refused", method: http.MethodPut, path: "/api/v1/apps/web", body: `{"image":"img:v1","port":80,"env":{"K":"v"},"secret_env":["K"]}`,
			wantStatus: http.StatusConflict, wantBody: "both a plain variable and a secret"},
		{name: "create with a key in both is refused", method: http.MethodPost, path: "/api/v1/apps", body: `{"name":"web2","image":"img:v1","port":80,"env":{"K":"v"},"secret_env":["K"]}`,
			wantStatus: http.StatusBadRequest, wantBody: "both a plain variable and a secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setter := &fakeSecretSetter{err: tt.setterErr, locked: tt.locked}
			rt, db := newTestRouterWithSecrets(t, setter)
			cookie := loginTestSession(t, rt, db)
			seedDualCopyApp(t, db, tt.env, tt.secrets...)

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, tt.method, tt.path, tt.body))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("body = %s, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
			got, err := db.GetDesiredService(context.Background(), "web")
			if err != nil {
				t.Fatal(err)
			}
			_, plain := got.Env["K"]
			declared := len(envSecretOverlap(map[string]string{"K": ""}, store.SecretEnvNames(got.SecretEnv))) > 0
			if plain != tt.wantPlain {
				t.Errorf("plain copy present = %v, want %v", plain, tt.wantPlain)
			}
			if declared != tt.wantSecret {
				t.Errorf("secret declared = %v, want %v", declared, tt.wantSecret)
			}
			if plain && declared {
				t.Errorf("key K ended up as both plain and secret")
			}
		})
	}
}

func TestDualCopyDoctorListsExistingOverlap(t *testing.T) {
	rt, db := newTestRouterWithSecrets(t, &fakeSecretSetter{})
	seedDualCopyApp(t, db, map[string]string{"K": "v"})
	// The store refuses a new overlap, so write the legacy row directly.
	refs, err := json.Marshal(store.SecretEnvRefsFromNames([]string{"K"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), `UPDATE desired_services SET secret_env = ? WHERE name = 'web'`, string(refs)); err != nil {
		t.Fatal(err)
	}
	res := rt.doctorCheckEnvDualCopy(context.Background())
	if res.Status != doctorStatusWarn || !strings.Contains(res.Message, "apps env unset web K") {
		t.Fatalf("got %+v", res)
	}
}

func TestNewEnvSecretOverlapIgnoresOldDualCopy(t *testing.T) {
	old := map[string]string{"A": "1"}
	got := newEnvSecretOverlap(map[string]string{"A": "1", "B": "2"}, []string{"A", "B"}, old, []string{"A"})
	if len(got) != 1 || got[0] != "B" {
		t.Fatalf("got %v, want [B]", got)
	}
}
