package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	cfg, _ := json.Marshal(map[string]any{"base_url": base, "stream": "ops", "allow_http": true})
	return pluginkit.Call{Event: pluginkittest.Event(), Config: cfg, Secret: secret}
}

func TestContract(t *testing.T) {
	pluginkittest.CheckContract(t, newPlugin(), "plugin.yaml")
}

func TestSendDelivers(t *testing.T) {
	var form map[string][]string
	var user, pass string
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		user, pass, _ = r.BasicAuth()
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"result":"success","id":1}`))
	}))
	defer srv.Close()
	setup(t)
	if r := send(context.Background(), call(srv.URL, "bot@zulip.example.com:KEY123")); !r.Delivered {
		t.Fatalf("got %+v", r)
	}
	if path != "/api/v1/messages" || user != "bot@zulip.example.com" || pass != "KEY123" {
		t.Fatalf("path=%s user=%s pass=%s", path, user, pass)
	}
	if form["to"][0] != "ops" || form["topic"][0] != "alerts" || !strings.Contains(form["content"][0], "Disk almost full") {
		t.Fatalf("form = %v", form)
	}
}

func TestSendErrorsMapped(t *testing.T) {
	pluginkittest.RunErrorCases(t, func(serverURL string) pluginkit.Result {
		setup(t)
		return send(context.Background(), call(serverURL, "bot@zulip.example.com:KEY123"))
	})
}

func TestSendConfigAndCredentialErrors(t *testing.T) {
	setup(t)
	tests := []struct {
		name string
		c    pluginkit.Call
	}{
		{"no stream", pluginkit.Call{Event: pluginkittest.Event(), Config: json.RawMessage(`{"base_url":"https://z.example.com"}`), Secret: "a:b"}},
		{"http without opt in", pluginkit.Call{Event: pluginkittest.Event(), Config: json.RawMessage(`{"base_url":"http://z.example.com","stream":"s"}`), Secret: "a:b"}},
		{"malformed credential", call("https://z.example.com", "no-colon")},
		{"unknown field", pluginkit.Call{Event: pluginkittest.Event(), Config: json.RawMessage(`{"x":1}`), Secret: "a:b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if r := send(context.Background(), tc.c); r.Delivered || r.Retryable || r.Detail == "" {
				t.Fatalf("got %+v", r)
			}
		})
	}
}
