package api

import (
	"net/http"
	"testing"
)

const iacProjectOnlyDoc = `version: 1
kind: Project
metadata: {name: shop}
spec:
  env: {REGION: eu}
`

func TestIaCApplyRefusesStackSourcePrune(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	if code, out := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil); code != http.StatusOK {
		t.Fatalf("stack apply = %d %+v", code, out)
	}
	tok := seedMatrixToken(t, db, "writer", []string{AbilityRead, AbilityWrite})
	for _, src := range []string{"shop-prod", "SHOP-PROD"} {
		body := iacRequestBody(t, iacProjectOnlyDoc, map[string]any{"source": src, "prune": true})
		if code := tokenPost(t, rt, tok, "/api/v1/apply", body, nil); code != http.StatusConflict {
			t.Fatalf("legacy prune with source %q = %d, want 409", src, code)
		}
	}
	if _, err := db.GetDesiredService(t.Context(), "web"); err != nil {
		t.Fatalf("stack app was deleted: %v", err)
	}
}

func TestIaCApplyRefusesStackOwnedUpdate(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	if code, _ := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.0.0"), nil); code != http.StatusOK {
		t.Fatalf("stack apply = %d", code)
	}
	body := iacRequestBody(t, stackTestDocs("ghcr.io/acme/web:6.6.6"), map[string]any{"source": "legacy-ci"})
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apply", body), nil); code != http.StatusConflict {
		t.Fatalf("legacy update of stack app = %d, want 409", code)
	}
	if svc, err := db.GetDesiredService(t.Context(), "web"); err != nil || svc.Image != "ghcr.io/acme/web:1.0.0" {
		t.Fatalf("stack app changed: %+v %v", svc, err)
	}
}

func TestIaCApplyLegacySourcePruneStillWorks(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	body := iacRequestBody(t, iacTestDocs, map[string]any{"source": "legacy-ci"})
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apply", body), nil); code != http.StatusOK {
		t.Fatalf("legacy apply = %d", code)
	}
	body = iacRequestBody(t, iacProjectOnlyDoc, map[string]any{"source": "legacy-ci", "prune": true})
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apply", body), nil); code != http.StatusOK {
		t.Fatalf("legacy prune = %d", code)
	}
	if _, err := db.GetDesiredService(t.Context(), "web"); err == nil {
		t.Fatal("legacy prune should delete the app its source created")
	}
}
