package alerting

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendChatTextMatrixAndZulip(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotMethod, gotPath, gotAuth, gotBody = r.Method, r.URL.EscapedPath(), r.Header.Get("Authorization"), string(b)
		if u, p, ok := r.BasicAuth(); ok {
			gotAuth = u + ":" + p
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := sendChatText(context.Background(), srv.Client(), NotifyMatrix, srv.URL+"/?room=!abc:example.com&token=tok", "hello")
	if err != nil {
		t.Fatalf("matrix send: %v", err)
	}
	if gotMethod != http.MethodPut || !strings.HasPrefix(gotPath, "/_matrix/client/v3/rooms/%21abc:example.com/send/m.room.message/") || gotAuth != "Bearer tok" || !strings.Contains(gotBody, `"body":"hello"`) {
		t.Fatalf("matrix request wrong: %s %s %q %s", gotMethod, gotPath, gotAuth, gotBody)
	}

	zulip := strings.Replace(srv.URL, "http://", "http://bot%40example.com:key@", 1) + "/?stream=ops"
	if err := sendChatText(context.Background(), srv.Client(), NotifyZulip, zulip, "hi there"); err != nil {
		t.Fatalf("zulip send: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/messages" || gotAuth != "bot@example.com:key" ||
		!strings.Contains(gotBody, "to=ops") || !strings.Contains(gotBody, "topic=alerts") || !strings.Contains(gotBody, "content=hi+there") {
		t.Fatalf("zulip request wrong: %s %s %q %s", gotMethod, gotPath, gotAuth, gotBody)
	}
}

func TestChatTargetsRejectIncomplete(t *testing.T) {
	if _, err := parseMatrixTarget("https://m.example.com/?room=abc&token=t"); err == nil {
		t.Fatal("matrix room without ! must fail")
	}
	if _, err := parseZulipTarget("https://z.example.com/?stream=ops"); err == nil {
		t.Fatal("zulip without credentials must fail")
	}
}
