package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestModelKeysRoutes_RequireAuthAndAbilities(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/models/chat/keys"},
		{http.MethodPost, "/api/v1/models/chat/keys"},
		{http.MethodDelete, "/api/v1/models/chat/keys/k1"},
		{http.MethodPost, "/api/v1/models/chat/keys/k1/rotate"},
		{http.MethodGet, "/api/v1/models/chat/usage"},
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
		{"read lists keys", http.MethodGet, "/api/v1/models/x/keys", "ro-secret", http.StatusNotFound},
		{"read reads usage", http.MethodGet, "/api/v1/models/x/usage", "ro-secret", http.StatusNotFound},
		{"read cannot create", http.MethodPost, "/api/v1/models/x/keys", "ro-secret", http.StatusForbidden},
		{"write cannot create (needs write:sensitive)", http.MethodPost, "/api/v1/models/x/keys", "rw-secret", http.StatusForbidden},
		{"write cannot rotate (needs write:sensitive)", http.MethodPost, "/api/v1/models/x/keys/k/rotate", "rw-secret", http.StatusForbidden},
		{"read cannot revoke", http.MethodDelete, "/api/v1/models/x/keys/k", "ro-secret", http.StatusForbidden},
		{"write can revoke", http.MethodDelete, "/api/v1/models/x/keys/k", "rw-secret", http.StatusNotFound},
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

func TestModelKeys_Flow(t *testing.T) {
	rt, db := newModelsTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models", `{"name":"chat","engine":"ollama","model":"llama3.1:8b"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create model = %d %s", rec.Code, rec.Body.String())
	}

	rec := doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/chat/keys",
		`{"name":"ci","rpm":30,"tpm":5000,"max_parallel":2,"allow_paths":["/v1/chat/completions"],"allow_models":["llama3.1:8b"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create key = %d %s", rec.Code, rec.Body.String())
	}
	var created createdModelKeyResource
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if !strings.HasPrefix(created.APIKey, "lr-") || created.KeyPrefix != created.APIKey[:8] || created.RPM != 30 || created.Status != "active" {
		t.Errorf("created = %+v", created)
	}

	rec = doModels(t, rt, cookie, http.MethodGet, "/api/v1/models/chat/keys", "")
	if strings.Contains(rec.Body.String(), created.APIKey) || strings.Contains(rec.Body.String(), "key_hash") {
		t.Errorf("list must never return key material: %s", rec.Body.String())
	}
	var list []modelKeyResource
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 2 || list[0].Name != "default" {
		t.Errorf("list = %+v", list)
	}

	if rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/chat/keys", `{"name":"ci"}`); rec.Code != http.StatusConflict {
		t.Errorf("duplicate name = %d", rec.Code)
	}
	if rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/chat/keys", `{"name":"bad","allow_paths":["/api/pull"]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("admin path allowed = %d", rec.Code)
	}
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	if rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/chat/keys", `{"name":"old","expires_at":"`+past+`"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("past expiry = %d", rec.Code)
	}

	rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/chat/keys/"+created.ID+"/rotate", `{"grace_seconds":600}`)
	var rotated createdModelKeyResource
	_ = json.Unmarshal(rec.Body.Bytes(), &rotated)
	if rec.Code != http.StatusOK || rotated.APIKey == "" || rotated.APIKey == created.APIKey || rotated.Name != "ci" || rotated.RPM != 30 {
		t.Fatalf("rotate = %d %s", rec.Code, rec.Body.String())
	}
	if rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/chat/keys/"+created.ID+"/rotate", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("re-rotating a retired key = %d", rec.Code)
	}
	if rec = doModels(t, rt, cookie, http.MethodPost, "/api/v1/models/chat/keys/"+rotated.ID+"/rotate", `{"grace_seconds":-1}`); rec.Code != http.StatusBadRequest {
		t.Errorf("negative grace = %d", rec.Code)
	}

	if rec = doModels(t, rt, cookie, http.MethodDelete, "/api/v1/models/chat/keys/"+rotated.ID, ""); rec.Code != http.StatusNoContent {
		t.Errorf("revoke = %d", rec.Code)
	}
	if rec = doModels(t, rt, cookie, http.MethodDelete, "/api/v1/models/chat/keys/missing", ""); rec.Code != http.StatusNotFound {
		t.Errorf("revoke missing = %d", rec.Code)
	}

	if err := db.AddModelUsage(context.Background(), []store.ModelUsage{{ModelName: "chat", KeyID: created.ID, HourStart: time.Now().UTC().Truncate(time.Hour), Requests: 4, InputTokens: 10, OutputTokens: 20}}); err != nil {
		t.Fatal(err)
	}
	rec = doModels(t, rt, cookie, http.MethodGet, "/api/v1/models/chat/usage?since=6h", "")
	var usage struct {
		Totals struct {
			Requests    int64 `json:"requests"`
			InputTokens int64 `json:"input_tokens"`
		} `json:"totals"`
		Keys []struct {
			Name     string `json:"name"`
			Requests int64  `json:"requests"`
		} `json:"keys"`
		Note string `json:"note"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &usage)
	if rec.Code != http.StatusOK || usage.Totals.Requests != 4 || usage.Totals.InputTokens != 10 || usage.Note == "" || len(usage.Keys) != 3 {
		t.Errorf("usage = %d %s", rec.Code, rec.Body.String())
	}
	if rec = doModels(t, rt, cookie, http.MethodGet, "/api/v1/models/chat/usage?since=abc", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad since = %d", rec.Code)
	}
}
