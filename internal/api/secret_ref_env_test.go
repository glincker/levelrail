package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func putRefAs(rt *Router, tok, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/vault-env/API_KEY", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestSecretRefEnvNeedsRootAndIsStoredAsReference(t *testing.T) {
	rt, db := newTestRouter(t)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	writer := seedMatrixToken(t, db, "ws2", []string{AbilityWrite})
	root := seedMatrixToken(t, db, "root2", []string{AbilityRoot})
	body := `{"ref":"secret://prod/apps/web#api"}`

	if rec := putRefAs(rt, writer, body); rec.Code != http.StatusForbidden {
		t.Fatalf("non root reference = %d, want 403", rec.Code)
	}
	if rec := putRefAs(rt, root, `{"ref":"secret://Bad/../x#k"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid reference = %d, want 400", rec.Code)
	}
	rec := putRefAs(rt, root, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("root reference = %d: %s", rec.Code, rec.Body.String())
	}
	saved, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if got := saved.VaultEnv["API_KEY"].SecretRef(); got != "secret://prod/apps/web#api" {
		t.Fatalf("stored reference = %q", got)
	}
	// The legacy form is unchanged and needs no root.
	if rec := putRefAs(rt, writer, `{"path":"a","key":"b"}`); rec.Code != http.StatusOK {
		t.Fatalf("legacy vault form = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLiftEnvSecretRefs(t *testing.T) {
	env, vault, err := liftEnvSecretRefs(map[string]string{"A": "secret://prod/p#k", "B": "plain"}, nil)
	if err != nil || len(env) != 1 || env["B"] != "plain" || vault["A"].Conn != "prod" {
		t.Fatalf("env=%v vault=%v err=%v", env, vault, err)
	}
	if _, _, err := liftEnvSecretRefs(map[string]string{"A": "secret://bad"}, nil); err == nil {
		t.Fatal("a malformed reference must fail")
	} else if strings.Contains(err.Error(), "secret://bad") {
		t.Fatalf("error echoes the input: %v", err)
	}
}

// The structured conn form of PUT vault-env is root only like the ref string form,
// and a traversal path never reaches storage for any caller.
func TestAuthzMatrix_VaultEnvConnFormIsRootOnly(t *testing.T) {
	rt, db := newTestRouter(t)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	writer := seedMatrixToken(t, db, "ws3", []string{AbilityWrite})
	root := seedMatrixToken(t, db, "root3", []string{AbilityRoot})
	if rec := putRefAs(rt, writer, `{"conn":"prod","path":"apps/web","key":"api"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("writer conn form = %d, want 403", rec.Code)
	}
	for _, tok := range []string{writer, root} {
		if rec := putRefAs(rt, tok, `{"conn":"prod","path":"../etc","key":"api"}`); rec.Code != http.StatusBadRequest {
			t.Fatalf("traversal path = %d, want 400", rec.Code)
		}
	}
}
