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

func call(secret string) pluginkit.Call {
	return pluginkit.Call{Event: pluginkittest.Event(), Config: json.RawMessage(`{}`), Secret: secret}
}

func TestContract(t *testing.T) {
	pluginkittest.CheckContract(t, newPlugin(), "plugin.yaml")
}

func TestSendDelivers(t *testing.T) {
	var got map[string]any
	var key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		key = r.URL.Query().Get("key")
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	setup(t, u.Hostname())
	if r := send(context.Background(), call(srv.URL+"/cgi-bin/webhook/send?key=KEY1")); !r.Delivered {
		t.Fatalf("got %+v", r)
	}
	text := got["text"].(map[string]any)["content"].(string)
	if got["msgtype"] != "text" || !strings.Contains(text, "Disk almost full") || key != "KEY1" {
		t.Fatalf("body = %v key=%s", got, key)
	}
}

func TestSendRejectionIsPermanent(t *testing.T) {
	srv := httptest.NewServer(pluginkittest.Status(200, `{"errcode":93000,"errmsg":"invalid webhook url, hint: KEY1"}`))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	setup(t, u.Hostname())
	r := send(context.Background(), call(srv.URL+"/cgi-bin/webhook/send?key=KEY1"))
	if r.Delivered || r.Retryable || !strings.Contains(r.Detail, "93000") || strings.Contains(r.Detail, "KEY1") {
		t.Fatalf("got %+v", r)
	}
}

func TestSendErrorsMapped(t *testing.T) {
	pluginkittest.RunErrorCases(t, func(serverURL string) pluginkit.Result {
		u, _ := url.Parse(serverURL)
		setup(t, u.Hostname())
		return send(context.Background(), call(serverURL+"/cgi-bin/webhook/send?key=KEY1"))
	})
}

func TestSendRejectsBadTarget(t *testing.T) {
	setup(t, "qyapi.weixin.qq.com")
	for _, secret := range []string{"", "https://evil.example.com/cgi-bin/webhook/send?key=K", "https://qyapi.weixin.qq.com/cgi-bin/webhook/send"} {
		if r := send(context.Background(), call(secret)); r.Delivered || r.Retryable {
			t.Fatalf("%q accepted: %+v", secret, r)
		}
	}
}
