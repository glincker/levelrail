package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

func TestBranchAPI_SchemaDiffDenyAndNotReady(t *testing.T) {
	rt, db, cookie, _ := newLiveBranchRouter(t)
	const plain = "schema-diff-read-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: "tok_sd_ro", Name: "ro", TokenHash: hashToken(plain), Abilities: []string{AbilityRead}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	const path = "/api/v1/databases/main/branches/b1/schema-diff"
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodGet, path, "", plain))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read only token = %d, want 403", rec.Code)
	}
	branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"b1"}`)
	rec = branchDo(t, rt, cookie, http.MethodGet, path, "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "branch_not_ready") {
		t.Fatalf("copying branch = %d %s", rec.Code, rec.Body.String())
	}
	if rec = branchDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/main/branches/nope/schema-diff", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing branch = %d", rec.Code)
	}
}
