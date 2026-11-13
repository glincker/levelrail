package api

import (
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
)

func TestStackErrorCodes(t *testing.T) {
	rt, _, cookie := stackFixture(t)
	if code, _ := stackApply(t, rt, cookie, stackTestDocs("ghcr.io/acme/web:1.2.3"), nil); code != http.StatusOK {
		t.Fatalf("seed apply: %d", code)
	}
	cases := []struct {
		name, method, path, body string
		status                   int
		code                     string
	}{
		{"unknown stack", http.MethodGet, "/api/v1/stacks/nope", "", http.StatusNotFound, "not_found"},
		{"bad revision number", http.MethodGet, "/api/v1/stacks/shop-prod/revisions/zero", "", http.StatusBadRequest, iac.CodeInvalidRequest},
		{"bad against", http.MethodGet, "/api/v1/stacks/shop-prod/revisions/1/diff?against=x", "", http.StatusBadRequest, iac.CodeInvalidRequest},
		{"bad body", http.MethodPost, "/api/v1/stacks", "{", http.StatusBadRequest, iac.CodeInvalidRequest},
		{"bad stack name", http.MethodPost, "/api/v1/stacks", `{"name":"Bad Name!","project":"shop"}`, http.StatusUnprocessableEntity, iac.CodeInvalidRequest},
		{"duplicate stack", http.MethodPost, "/api/v1/stacks", `{"name":"shop-prod","project":"shop"}`, http.StatusConflict, iac.CodeOwnershipConflict},
	}
	for _, c := range cases {
		var out iacErrorBody
		got := iacCall(t, rt, authedRequest(t, cookie, c.method, c.path, c.body), &out)
		if got != c.status || out.Code != c.code || out.Error == "" {
			t.Errorf("%s: status %d body %+v, want %d %s", c.name, got, out, c.status, c.code)
		}
	}
}
