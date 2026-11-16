package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestGitRotateKeepsUnsavedTokenAndMarksReconnect(t *testing.T) {
	t.Setenv(envGitRotateSaveWait, "1ms")
	fake := &credFakeGitLab{fakeGitLabAppClient: &fakeGitLabAppClient{}, rotateTok: "glpat-pending-secret", rotateExp: "2027-01-31"}
	rt, db, secrets, _ := newCredRouter(t, nil, fake)
	c := staticGitLabConn(t, db, secrets, "bea")

	secrets.setErr = errFakeSecretNotSet
	rec := postRotate(t, rt, db, c.ID)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "glpat-pending-secret") {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rt.gitConnHealth(t.Context(), c.ID); got.Status != store.GitConnNeedsReauthorize {
		t.Fatalf("health = %+v, want needs_reauthorize", got)
	}

	secrets.setErr = nil
	rec = postRotate(t, rt, db, c.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("retry status = %d body = %s", rec.Code, rec.Body.String())
	}
	if fake.rotated != 1 {
		t.Fatalf("host rotated %d times, the kept token must be reused", fake.rotated)
	}
	if v, _ := secrets.Resolve(t.Context(), c.SecretsKey, gitConnAccessTokenKey); v != "glpat-pending-secret" {
		t.Fatal("the kept token was not saved on retry")
	}
	if got := rt.gitConnHealth(t.Context(), c.ID); got.Status != store.GitConnOK {
		t.Fatalf("health after save = %+v", got)
	}
}
