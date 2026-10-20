package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/internal/store"
)

type stubEndpointChecker struct{ res models.EndpointProbe }

func (s stubEndpointChecker) Probe(context.Context, string, string, string) models.EndpointProbe {
	return s.res
}

func newEndpointTestRouter(t *testing.T, res models.EndpointProbe) (*Router, *store.DB) {
	t.Helper()
	t.Setenv(models.EndpointAllowPrivateEnv, "true")
	db := openTestDB(t)
	svc := models.NewService(db, nil, models.NewHostResolver("", nil), "")
	svc.SetEndpoints(models.EndpointPlumbing{Store: db, Checker: stubEndpointChecker{res}})
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithModels(svc)), db
}

var onlineProbe = models.EndpointProbe{State: store.EndpointOnline, Models: []string{"qwen2.5:0.5b"}}

func TestModelEndpointRoutes_RequireAuthAndAbilities(t *testing.T) {
	rt, db := newEndpointTestRouter(t, onlineProbe)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodPost, "/api/v1/model-endpoints/test"},
		{http.MethodPost, "/api/v1/models/mac/endpoint/test"},
		{http.MethodPut, "/api/v1/models/mac/endpoint/routing"},
		{http.MethodPut, "/api/v1/models/mac/endpoint/credential"},
	})
	for id, ab := range map[string]string{"ro": AbilityRead, "rw": AbilityWrite} {
		if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: id, Name: id, TokenHash: hashToken(id + "-secret"), Abilities: []string{ab}}); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name, method, target, token string
		want                        int
	}{
		{"read cannot probe", http.MethodPost, "/api/v1/model-endpoints/test", "ro-secret", http.StatusForbidden},
		{"read cannot test saved", http.MethodPost, "/api/v1/models/mac/endpoint/test", "ro-secret", http.StatusForbidden},
		{"read cannot set routing", http.MethodPut, "/api/v1/models/mac/endpoint/routing", "ro-secret", http.StatusForbidden},
		{"write cannot set credential (needs write:sensitive)", http.MethodPut, "/api/v1/models/mac/endpoint/credential", "rw-secret", http.StatusForbidden},
		{"write may set routing", http.MethodPut, "/api/v1/models/mac/endpoint/routing", "rw-secret", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, bearerRequest(tt.method, tt.target, "{}", tt.token))
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestModelEndpoints_Flow(t *testing.T) {
	rt, db := newEndpointTestRouter(t, onlineProbe)
	cookie := loginTestSession(t, rt, db)

	rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", `{"name":"mac","external":{"base_url":"http://100.64.0.7:11434","api_type":"ollama"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		modelResource
		APIKey string `json:"api_key"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if !strings.HasPrefix(created.APIKey, "lr-") || created.External == nil || created.External.Health.State != store.EndpointOnline ||
		len(created.External.Models) != 1 || !created.Status.Ready || created.Compute != models.ComputeExternal {
		t.Fatalf("created = %+v", created)
	}

	rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/model-endpoints/test", `{"base_url":"http://10.0.0.5:1234","api_type":"openai"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"state":"online"`) {
		t.Fatalf("test = %d %s", rec.Code, rec.Body.String())
	}
	rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/model-endpoints/test", `{"base_url":"http://169.254.169.254"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), models.DiagBlocked) || !strings.Contains(rec.Body.String(), "use_other_address") {
		t.Fatalf("metadata probe = %d %s", rec.Code, rec.Body.String())
	}
	if rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", `{"name":"evil","external":{"base_url":"http://169.254.169.254/latest"}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("metadata create = %d %s", rec.Code, rec.Body.String())
	}

	if rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", `{"name":"cloud","engine":"ollama","model":"llama3.1:8b"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create cloud = %d %s", rec.Code, rec.Body.String())
	}
	rec = doModels(t, rt, cookie, http.MethodPut, "/api/v1/models/mac/endpoint/routing", `{"fallback_model":"cloud","fallback_model_ref":"llama3.1:8b","latency_budget_ms":3000}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"fallback_model":"cloud"`) || !strings.Contains(rec.Body.String(), `"latency_budget_ms":3000`) {
		t.Fatalf("routing = %d %s", rec.Code, rec.Body.String())
	}
	if rec = doModels(t, rt, cookie, http.MethodPut, "/api/v1/models/mac/endpoint/routing", `{"fallback_model":"mac"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("self fallback = %d", rec.Code)
	}
	if rec = doModels(t, rt, cookie, http.MethodPut, "/api/v1/models/cloud/endpoint/routing", `{"fallback_model":"mac"}`); rec.Code != http.StatusNotFound {
		t.Errorf("routing on a non endpoint = %d", rec.Code)
	}
	if rec = doModels(t, rt, cookie, http.MethodPut, "/api/v1/models/mac/endpoint/credential", `{"credential":"sk-x"}`); rec.Code != http.StatusNotImplemented {
		t.Errorf("credential without secrets manager = %d %s", rec.Code, rec.Body.String())
	}
	if rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/mac/endpoint/test", ""); rec.Code != http.StatusOK {
		t.Errorf("saved test = %d %s", rec.Code, rec.Body.String())
	}
	if rec = doModels(t, rt, cookie, http.MethodDelete, "/api/v1/models/mac", ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete = %d", rec.Code)
	}
	if rec = doModels(t, rt, cookie, http.MethodGet, "/api/v1/models/mac", ""); rec.Code != http.StatusNotFound {
		t.Errorf("get after delete = %d", rec.Code)
	}
}

func TestModelEndpoints_TenantScope(t *testing.T) {
	rt, db := newEndpointTestRouter(t, onlineProbe)
	bootstrapTestAdmin(t, db)
	admin := loginTestSession(t, rt, db)
	for _, body := range []string{
		`{"name":"mac","external":{"base_url":"http://10.0.0.5:11434"}}`,
		`{"name":"private-cloud","engine":"ollama","model":"llama3.1:8b"}`,
	} {
		if rec := doModels(t, rt, admin, http.MethodPost, "/api/v1/models", body); rec.Code != http.StatusCreated {
			t.Fatalf("seed = %d %s", rec.Code, rec.Body.String())
		}
	}
	user := storeUserWithAbilitiesForTest(t, db, "scoped@example.com", []string{AbilityWrite, AbilityWriteSensitive})
	attachTestPolicy(t, db, "deny-private", "Deny", AbilityWrite, "model:private-cloud", store.PrincipalTypeUser, user.ID)
	attachTestPolicy(t, db, "deny-mac", "Deny", AbilityWrite, "model:locked", store.PrincipalTypeUser, user.ID)
	cookie := sessionCookieForTest(t, rt, user.ID)

	rec := doModels(t, rt, cookie, http.MethodPut, "/api/v1/models/mac/endpoint/routing", `{"fallback_model":"private-cloud"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unknown fallback_model") {
		t.Errorf("fallback to a model the caller cannot write = %d %s", rec.Code, rec.Body.String())
	}
	rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", `{"name":"mac2","external":{"base_url":"http://10.0.0.6:11434","fallback_model":"private-cloud"}}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("create with a forbidden fallback = %d %s", rec.Code, rec.Body.String())
	}
	if rec = doModels(t, rt, cookie, http.MethodPut, "/api/v1/models/private-cloud/endpoint/routing", `{}`); rec.Code != http.StatusForbidden {
		t.Errorf("routing on a denied model = %d", rec.Code)
	}
}
