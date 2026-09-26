package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/internal/store"
)

func hubFixture(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/models/acme/chat-GGUF" {
			_, _ = w.Write([]byte(`{"id":"acme/chat-GGUF","gated":false,"cardData":{"license":"mit"},"siblings":[{"rfilename":"chat-Q4_K_M.gguf","size":2000000000}]}`))
			return
		}
		http.Error(w, "no", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestModelPreflightRoute(t *testing.T) {
	t.Setenv("APP_HF_BASE_URL", hubFixture(t))
	t.Setenv("APP_NOTIFY_ALLOW_PRIVATE_NETWORKS", "true")
	rt, db := newModelsTestRouter(t)
	svc := models.NewService(db, nil, models.NewHostResolver("", nil), "")
	svc.SetHuggingFace(models.NewHFClient(models.LoadHFConfig(), nil, nil), models.LoadFitConfig())
	rt.models = svc
	if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: "t1", Name: "ro", TokenHash: hashToken("ro-secret"), Abilities: []string{AbilityRead}}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, body string
		want       int
		wantStatus string
	}{
		{"found", `{"repo":"acme/chat-GGUF","engine":"llamacpp"}`, http.StatusOK, "ok"},
		{"missing is a 200 with a status", `{"repo":"acme/none"}`, http.StatusOK, "not_found"},
		{"bad repo is a 400", `{"repo":"not a repo"}`, http.StatusBadRequest, ""},
		{"bad body is a 400", `{`, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodPost, "/api/v1/models/preflight", tt.body, "ro-secret"))
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
			if tt.wantStatus == "" {
				return
			}
			var got models.PreflightResult
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Status != tt.wantStatus {
				t.Errorf("status = %q err %v (%s)", got.Status, err, rec.Body.String())
			}
			if tt.wantStatus == "ok" && (got.License != "mit" || len(got.Quants) != 1 || got.Quants[0].Name != "Q4_K_M") {
				t.Errorf("result = %+v", got)
			}
		})
	}
}

func TestModelCacheRoutes_Guards(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	for id, ab := range map[string][]string{"ro": {AbilityRead}, "rw": {AbilityWrite}} {
		if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: id, Name: id, TokenHash: hashToken(id + "-secret"), Abilities: ab}); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name, method, target, token string
		want                        int
	}{
		{"read token reaches list (501 without a backend)", http.MethodGet, "/api/v1/model-cache", "ro-secret", http.StatusNotImplemented},
		{"read token cannot prune", http.MethodPost, "/api/v1/model-cache/prune", "ro-secret", http.StatusForbidden},
		{"write token cannot prune", http.MethodPost, "/api/v1/model-cache/prune", "rw-secret", http.StatusForbidden},
		{"preflight without hub config is 501", http.MethodPost, "/api/v1/models/preflight", "ro-secret", http.StatusNotImplemented},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, bearerRequest(tt.method, tt.target, `{}`, tt.token))
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}
