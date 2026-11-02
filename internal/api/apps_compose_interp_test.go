package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func composeWithEnv(env string) string {
	return "services:\n  web:\n    image: nginx:1.27\n    environment:\n" + env
}

func TestInterpEnv_ScopeAndRevisionText(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db)
	cookie := loginTestSession(t, rt, db)
	post := func(app, body string) (int, composeDeployResponse, string) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/"+app+"/compose", body))
		var got composeDeployResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		return rec.Code, got, rec.Body.String()
	}
	t.Setenv("CP_ONLY_SECRET_VAR", "from-control-plane")

	required := composeWithEnv("      DB_PASSWORD: \"${DB_PASSWORD:?set a database password}\"\n")
	code, _, body := post("appone", required)
	if code != http.StatusBadRequest || !strings.Contains(body, "set a database password") || !strings.Contains(body, `"missing_variables":[{"name":"DB_PASSWORD"`) {
		t.Fatalf("missing var: status %d body %s", code, body)
	}

	// Seed appone's own env, then the same text succeeds.
	if code, _, body := post("appone", composeWithEnv("      DB_PASSWORD: hunter2\n      PORT: \"9090\"\n")); code != http.StatusOK {
		t.Fatalf("seed: %d %s", code, body)
	}
	code, got, body := post("appone", composeWithEnv("      DB_PASSWORD: \"${DB_PASSWORD:?set a database password}\"\n      PORT: \"${PORT:-8080}\"\n      LEAK: \"${CP_ONLY_SECRET_VAR:-none}\"\n"))
	if code != http.StatusOK {
		t.Fatalf("redeploy: %d %s", code, body)
	}
	env := got.Services[0].Env
	if env["DB_PASSWORD"] == "" || env["PORT"] != "9090" {
		t.Errorf("env = %v, want app env values", env)
	}
	if env["LEAK"] != "none" {
		t.Errorf("control plane env leaked: %q", env["LEAK"])
	}

	// Another app does not see appone's env.
	code, other, body := post("apptwo", composeWithEnv("      PORT: \"${PORT:-8080}\"\n"))
	if code != http.StatusOK || other.Services[0].Env["PORT"] != "8080" {
		t.Errorf("other app: %d env=%v body=%s", code, other.Services, body)
	}
	if code, _, _ := post("apptwo", required); code != http.StatusBadRequest {
		t.Errorf("other app must not see appone DB_PASSWORD, status %d", code)
	}

	revs, err := db.ListComposeRevisions(context.Background(), "appone", 1)
	if err != nil || len(revs) == 0 {
		t.Fatalf("revisions: %v", err)
	}
	if !strings.Contains(revs[0].Compose, "${DB_PASSWORD:?set a database password}") || strings.Contains(revs[0].Compose, "hunter2") {
		t.Errorf("stored revision lost the original text: %s", revs[0].Compose)
	}
}
