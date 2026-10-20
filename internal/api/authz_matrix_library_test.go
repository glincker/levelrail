package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestAuthzMatrix_LibraryOnlyRoutesAreProtected(t *testing.T) {
	registered := map[string]bool{}
	for _, r := range loadMatrixRoutes(t) {
		registered[r.key()] = true
	}
	h := newMFAHarness(t)
	tok := seedMatrixToken(t, h.db, "ro", []string{AbilityRead})
	for key, reason := range libraryOnlyRoutes {
		if !registered[key] {
			t.Errorf("libraryOnlyRoutes entry %q is not a declared route, remove it", key)
			continue
		}
		method, pattern, _ := strings.Cut(key, " ")
		path := concretePath(pattern, "web")
		if got := matrixDo(h.rt, method, path, nil); got != http.StatusUnauthorized {
			t.Errorf("%s (%s): anonymous request -> %d, want 401", key, reason, got)
		}
		if got := matrixDo(h.rt, method, path, bearer(tok)); got != http.StatusForbidden && got != http.StatusUnauthorized {
			t.Errorf("%s (%s): read-only token -> %d, want 401 or 403", key, reason, got)
		}
		if rec := doJSON(t, h.rt, method, path, "{}", h.cookie); rec.Code == http.StatusMethodNotAllowed {
			t.Errorf("%s (%s): not served (405)", key, reason)
		}
	}
}
