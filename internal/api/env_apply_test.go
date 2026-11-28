package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func envApplyVersion(t *testing.T, body string) int {
	t.Helper()
	var got struct {
		EnvVersion int `json:"env_version"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	return got.EnvVersion
}

func TestEnvApply_DryRunThenApplyThenStaleConflict(t *testing.T) {
	f := newEnvFixture(t)
	const path = "/api/v1/apps/web/env/apply"

	dry := f.do(t, f.cookie, http.MethodPost, path, `{"dry_run":true,"set":{"NEW_KEY":"v1"},"secret_set":{"STRIPE_KEY":"`+storedSecretValue+`"}}`, http.StatusOK)
	assertNoSecretValues(t, "dry run", dry.Body.String())
	if !strings.Contains(dry.Body.String(), `"dry_run":true`) || !strings.Contains(dry.Body.String(), "NEW_KEY") {
		t.Fatalf("dry run body = %s", dry.Body.String())
	}
	svc, _ := f.db.GetDesiredService(t.Context(), "web")
	if _, ok := svc.Env["NEW_KEY"]; ok {
		t.Fatal("dry run wrote the env")
	}

	applied := f.do(t, f.cookie, http.MethodPost, path, `{"expected_version":`+envApplyInt(envApplyVersion(t, dry.Body.String()))+`,"set":{"NEW_KEY":"v1"},"secret_set":{"STRIPE_KEY":"`+storedSecretValue+`"}}`, http.StatusOK)
	assertNoSecretValues(t, "apply", applied.Body.String())
	next := envApplyVersion(t, applied.Body.String())
	if next < 1 {
		t.Fatalf("version after apply = %d", next)
	}

	stale := f.do(t, f.cookie, http.MethodPost, path, `{"expected_version":`+envApplyInt(next-1)+`,"set":{"OTHER":"x"}}`, http.StatusConflict)
	if !strings.Contains(stale.Body.String(), `"current_version":`+envApplyInt(next)) || !strings.Contains(stale.Body.String(), "OTHER") {
		t.Fatalf("conflict body = %s", stale.Body.String())
	}
	assertNoSecretValues(t, "conflict", stale.Body.String())
}

func TestEnvApply_RejectsSameKeyBothWaysAndNeedsSensitiveForSecrets(t *testing.T) {
	f := newEnvFixture(t)
	const path = "/api/v1/apps/web/env/apply"
	f.do(t, f.cookie, http.MethodPost, path, `{"set":{"K":"a"},"secret_set":{"K":"b"}}`, http.StatusBadRequest)
	f.do(t, f.cookie, http.MethodPost, path, `{}`, http.StatusBadRequest)

	tok := restoreGateToken(t, f, "apply1", []string{AbilityWrite})
	rec := f.serve(bearerRequest(http.MethodPost, path, `{"secret_set":{"K":"b"}}`, tok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("write-only secret_set = %d, want 403", rec.Code)
	}
	rec = f.serve(bearerRequest(http.MethodPost, path, `{"set":{"PLAIN_ONLY":"1"}}`, tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("write-only plain apply = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEnvApply_ConvertMovesValueAtomically(t *testing.T) {
	f := newEnvFixture(t)
	const path = "/api/v1/apps/web/env/apply"
	f.do(t, f.cookie, http.MethodPost, path, `{"convert":["PORT"]}`, http.StatusBadRequest)
	v, err := f.db.EnvVersion(t.Context(), "web")
	if err != nil {
		t.Fatal(err)
	}
	f.do(t, f.cookie, http.MethodPost, path, `{"expected_version":`+envApplyInt(v)+`,"convert":["PORT"]}`, http.StatusOK)
	svc, _ := f.db.GetDesiredService(t.Context(), "web")
	if _, ok := svc.Env["PORT"]; ok || len(svc.SecretEnv) == 0 {
		t.Fatalf("convert left env=%v secrets=%v", svc.Env, svc.SecretEnv)
	}
}

func envApplyInt(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
