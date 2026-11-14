package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestComposeDown_RefusesProtectedApp(t *testing.T) {
	t.Parallel()
	rt := &Router{apps: &fakeProtectApps{p: &store.AppProtection{Reason: "demo week"}}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/stack/compose", nil)
	r.SetPathValue("name", "stack")
	rt.handleComposeDown(w, r)
	if w.Code != http.StatusLocked || !strings.Contains(w.Body.String(), "app is protected: demo week") {
		t.Fatalf("got %d %s, want 423 with the protection reason", w.Code, w.Body.String())
	}
}

func TestComposeDigest_MatchesStoredBody(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/stack/compose", stackV1, http.StatusOK)

	var stack composeStackResource
	_ = json.Unmarshal(composeDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/stack/compose", "", http.StatusOK), &stack)
	sum := sha256.Sum256([]byte(stackV1))
	if want := "sha256:" + hex.EncodeToString(sum[:]); stack.ComposeDigest != want {
		t.Fatalf("compose_digest = %q, want %q", stack.ComposeDigest, want)
	}
}
