package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit"
	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit/pluginkittest"
)

func setup(t *testing.T) {
	t.Helper()
	old := httpc
	httpc.Sleep = pluginkittest.NoSleep
	t.Cleanup(func() { httpc = old })
}

func call(base, secret string) pluginkit.Call {
	cfg, _ := json.Marshal(map[string]any{"homeserver": base, "room_id": "!abc:example.com", "allow_http": true})
	return pluginkit.Call{Event: pluginkittest.Event(), Config: cfg, Secret: secret}
}

func TestContract(t *testing.T) {
	pluginkittest.CheckContract(t, newPlugin(), "plugin.yaml")
}

func TestSendDelivers(t *testing.T) {
	var method, escapedPath, auth string
	var body map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, escapedPath, auth = r.Method, r.URL.EscapedPath(), r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = w.Write([]byte(`{"event_id":"$e"}`))
	}))
	defer srv.Close()
	setup(t)
	if r := send(context.Background(), call(srv.URL, "syt_token")); !r.Delivered {
		t.Fatalf("got %+v", r)
	}
	if method != http.MethodPut || auth != "Bearer syt_token" || !strings.HasPrefix(escapedPath, "/_matrix/client/v3/rooms/%21abc:example.com/send/m.room.message/") {
		t.Fatalf("method=%s path=%s auth=%s", method, escapedPath, auth)
	}
	if body["msgtype"] != "m.text" || !strings.Contains(body["body"], "Disk almost full") {
		t.Fatalf("body = %v", body)
	}
}

func TestRetryReusesTransactionID(t *testing.T) {
	var calls atomic.Int32
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if calls.Add(1) == 1 {
			w.WriteHeader(502)
			return
		}
		_, _ = w.Write([]byte(`{"event_id":"$e"}`))
	}))
	defer srv.Close()
	setup(t)
	if r := send(context.Background(), call(srv.URL, "syt_token")); !r.Delivered {
		t.Fatalf("got %+v", r)
	}
	if len(paths) != 2 || paths[0] != paths[1] {
		t.Fatalf("a retry must reuse the transaction id: %v", paths)
	}
}

func TestSendErrorsMapped(t *testing.T) {
	pluginkittest.RunErrorCases(t, func(serverURL string) pluginkit.Result {
		setup(t)
		return send(context.Background(), call(serverURL, "syt_token"))
	})
}

func TestMatrixRateLimitBodyHonoured(t *testing.T) {
	srv := httptest.NewServer(pluginkittest.Status(429, `{"errcode":"M_LIMIT_EXCEEDED","retry_after_ms":60000}`))
	defer srv.Close()
	setup(t)
	r := send(context.Background(), call(srv.URL, "syt_token"))
	if r.Delivered || !r.Retryable || !strings.Contains(r.Detail, "1m0s") {
		t.Fatalf("got %+v", r)
	}
}

func TestSendConfigErrors(t *testing.T) {
	setup(t)
	for name, c := range map[string]pluginkit.Call{
		"public room alias": {Event: pluginkittest.Event(), Config: json.RawMessage(`{"homeserver":"https://m.example.com","room_id":"#ops:example.com"}`), Secret: "t"},
		"no token":          call("https://m.example.com", ""),
	} {
		t.Run(name, func(t *testing.T) {
			if r := send(context.Background(), c); r.Delivered || r.Retryable {
				t.Fatalf("got %+v", r)
			}
		})
	}
}
