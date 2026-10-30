package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStackApproveRejectAndPendingExit(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotPath, gotBody = r.URL.Path, string(b)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/apply"):
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"stack":"shop","pending_approval":true,"expires_at":"2026-10-26T10:00:00Z","revision":{"number":5,"status":"pending_approval","plan_hash":"h","actor":"ci","created_at":"2026-10-25T10:00:00Z"}}`))
		case strings.HasSuffix(r.URL.Path, "/approve"):
			_, _ = w.Write([]byte(`{"stack":"shop","revision":{"number":5,"status":"applied","plan_hash":"h","actor":"ci","created_at":"2026-10-25T10:00:00Z"},"result":{"applied":1}}`))
		default:
			_, _ = w.Write([]byte(`{"stack":"shop","revision":{"number":5,"status":"failed","plan_hash":"h","actor":"ci","created_at":"2026-10-25T10:00:00Z"},"reason":"no"}`))
		}
	}))
	defer srv.Close()

	code, out, _ := runIaC([]string{"stack", "apply", "shop", "-f", stackTestDir(t), "--yes", "--api-url", srv.URL}, nil)
	if code != iacExitPending || !strings.Contains(out, "awaiting approval") {
		t.Fatalf("pending apply: %d %s", code, out)
	}
	code, out, _ = runIaC([]string{"stack", "approve", "shop", "5", "--secret", "TOKEN=env:SHOP_TOKEN", "--api-url", srv.URL}, map[string]string{"SHOP_TOKEN": "s3cret-value"})
	if code != exitOK || gotPath != "/api/v1/stacks/shop/revisions/5/approve" || !strings.Contains(gotBody, `"TOKEN":"s3cret-value"`) || !strings.Contains(out, "revision 5 approved") {
		t.Fatalf("approve: %d %s %s %s", code, gotPath, gotBody, out)
	}
	code, _, _ = runIaC([]string{"stack", "reject", "shop", "5", "--reason", "no", "--api-url", srv.URL}, nil)
	if code != exitOK || gotPath != "/api/v1/stacks/shop/revisions/5/reject" || !strings.Contains(gotBody, `"reason":"no"`) {
		t.Fatalf("reject: %d %s %s", code, gotPath, gotBody)
	}
}
