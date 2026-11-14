package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
)

func TestIaCErrorCodes(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	bad := "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n    prot: 1\n"
	var out iacErrorBody
	code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apply/plan", iacRequestBody(t, bad, nil)), &out)
	if code != http.StatusUnprocessableEntity || out.Code != iac.CodeInvalidDocument || out.Error == "" || len(out.Issues) == 0 {
		t.Fatalf("invalid document: %d %+v", code, out)
	}
	for _, is := range out.Issues {
		if is.Path == "" || is.Code == "" || is.Message == "" {
			t.Fatalf("issue lacks path, code or message: %+v", is)
		}
	}
	if out.Issues[0].Code != iac.CodeSchemaUnknownField {
		t.Fatalf("issue code = %q", out.Issues[0].Code)
	}
	out = iacErrorBody{}
	code = iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apply", iacRequestBody(t, iacTestDocs, map[string]any{"prune": true})), &out)
	if code != http.StatusBadRequest || out.Code != iac.CodeInvalidRequest || len(out.Issues) != 0 {
		t.Fatalf("prune without source: %d %+v", code, out)
	}
	if strings.Contains(iac.CodeInvalidRequest, " ") {
		t.Fatal("codes must be stable tokens")
	}
}
