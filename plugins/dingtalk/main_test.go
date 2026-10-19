package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit"
	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit/pluginkittest"
)

func setup(t *testing.T, host string) {
	t.Helper()
	oldHost, oldHTTP, oldC := webhookHost, allowHTTP, httpc
	webhookHost, allowHTTP = host, true
	httpc.Sleep = pluginkittest.NoSleep
	t.Cleanup(func() { webhookHost, allowHTTP, httpc = oldHost, oldHTTP, oldC })
}

func call(secret, cfg string) pluginkit.Call {
	return pluginkit.Call{Event: pluginkittest.Event(), Config: json.RawMessage(cfg), Secret: secret}
}

func TestContract(t *testing.T) {
	pluginkittest.CheckContract(t, newPlugin(), "plugin.yaml")
}

func TestSendDelivers(t *testing.T) {
	var got map[string]any
	var token string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		token = r.URL.Query().Get("access_token")
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	setup(t, u.Hostname())
	if r := send(context.Background(), call(srv.URL+"/robot/send?access_token=TOK", `{"keyword":"ALERT"}`)); !r.Delivered {
		t.Fatalf("got %+v", r)
	}
	text := got["text"].(map[string]any)["content"].(string)
	if got["msgtype"] != "text" || !strings.HasPrefix(text, "ALERT ") || !strings.Contains(text, "Disk almost full") || token != "TOK" {
		t.Fatalf("body = %v token=%s", got, token)
	}
}

func TestSendRejectionIsPermanent(t *testing.T) {
	srv := httptest.NewServer(pluginkittest.Status(200, `{"errcode":310000,"errmsg":"keywords not in content TOK"}`))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	setup(t, u.Hostname())
	r := send(context.Background(), call(srv.URL+"/robot/send?access_token=TOK", `{}`))
	if r.Delivered || r.Retryable || !strings.Contains(r.Detail, "310000") || strings.Contains(r.Detail, "TOK") {
		t.Fatalf("got %+v", r)
	}
}

func TestSendErrorsMapped(t *testing.T) {
	pluginkittest.RunErrorCases(t, func(serverURL string) pluginkit.Result {
		u, _ := url.Parse(serverURL)
		setup(t, u.Hostname())
		return send(context.Background(), call(serverURL+"/robot/send?access_token=TOK", `{}`))
	})
}

func TestSendRejectsBadTarget(t *testing.T) {
	setup(t, "oapi.dingtalk.com")
	for _, secret := range []string{"", "https://evil.example.com/robot/send?access_token=T", "https://oapi.dingtalk.com/robot/send", "https://oapi.dingtalk.com/other?access_token=T"} {
		if r := send(context.Background(), call(secret, `{}`)); r.Delivered || r.Retryable {
			t.Fatalf("%q accepted: %+v", secret, r)
		}
	}
}
