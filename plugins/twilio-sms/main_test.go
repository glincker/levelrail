package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit"
	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit/pluginkittest"
)

var sid = "AC" + strings.Repeat("a", 32)

func setup(t *testing.T, base string) {
	t.Helper()
	oldBase, oldC := apiBase, httpc
	apiBase = base
	httpc.Sleep = pluginkittest.NoSleep
	t.Cleanup(func() { apiBase, httpc = oldBase, oldC })
}

func call(cfg map[string]string, secret string) pluginkit.Call {
	b, _ := json.Marshal(cfg)
	return pluginkit.Call{Event: pluginkittest.Event(), Config: b, Secret: secret}
}

func goodCfg() map[string]string {
	return map[string]string{"account_sid": sid, "from": "+15550001111", "to": "+15552223333"}
}

func TestContract(t *testing.T) {
	pluginkittest.CheckContract(t, newPlugin(), "plugin.yaml")
}

func TestSendDelivers(t *testing.T) {
	var form url.Values
	var user, pass, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form, path = r.PostForm, r.URL.Path
		user, pass, _ = r.BasicAuth()
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"sid":"SM1"}`))
	}))
	defer srv.Close()
	setup(t, srv.URL)
	if r := send(context.Background(), call(goodCfg(), "auth-token")); !r.Delivered {
		t.Fatalf("got %+v", r)
	}
	if path != "/2010-04-01/Accounts/"+sid+"/Messages.json" || user != sid || pass != "auth-token" {
		t.Fatalf("path=%s user=%s", path, user)
	}
	if form.Get("To") != "+15552223333" || form.Get("From") != "+15550001111" || !strings.Contains(form.Get("Body"), "Disk almost full") {
		t.Fatalf("form = %v", form)
	}
}

func TestMessagingServiceReplacesFrom(t *testing.T) {
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		w.WriteHeader(201)
	}))
	defer srv.Close()
	setup(t, srv.URL)
	cfg := map[string]string{"account_sid": sid, "messaging_service_sid": "MG0123456789abcdef0123456789abcdef", "to": "+15552223333"}
	if r := send(context.Background(), call(cfg, "auth-token")); !r.Delivered {
		t.Fatalf("got %+v", r)
	}
	if form.Get("MessagingServiceSid") == "" || form.Get("From") != "" {
		t.Fatalf("form = %v", form)
	}
}

func TestSendErrorsMapped(t *testing.T) {
	pluginkittest.RunErrorCases(t, func(serverURL string) pluginkit.Result {
		setup(t, serverURL)
		return send(context.Background(), call(goodCfg(), "auth-token"))
	})
}

func TestSendValidation(t *testing.T) {
	setup(t, "http://127.0.0.1:1")
	tests := map[string]pluginkit.Call{
		"bad sid":    call(map[string]string{"account_sid": "nope", "from": "+15550001111", "to": "+15552223333"}, "t"),
		"bad to":     call(map[string]string{"account_sid": sid, "from": "+15550001111", "to": "5552223333"}, "t"),
		"no sender":  call(map[string]string{"account_sid": sid, "to": "+15552223333"}, "t"),
		"no token":   call(goodCfg(), ""),
		"unknown":    {Event: pluginkittest.Event(), Config: json.RawMessage(`{"x":1}`), Secret: "t"},
		"empty body": {Event: pluginkittest.Event(), Config: json.RawMessage(`{}`), Secret: "t"},
	}
	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			if r := send(context.Background(), c); r.Delivered || r.Retryable || r.Detail == "" {
				t.Fatalf("got %+v", r)
			}
		})
	}
}
