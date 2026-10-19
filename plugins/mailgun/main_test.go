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

func setup(t *testing.T, base string) {
	t.Helper()
	oldBases, oldC := apiBases, httpc
	apiBases = map[string]string{"us": base, "eu": base + "/eu"}
	httpc.Sleep = pluginkittest.NoSleep
	t.Cleanup(func() { apiBases, httpc = oldBases, oldC })
}

func call(cfg map[string]string, secret string) pluginkit.Call {
	b, _ := json.Marshal(cfg)
	return pluginkit.Call{Event: pluginkittest.Event(), Config: b, Secret: secret}
}

func goodCfg() map[string]string {
	return map[string]string{"domain": "mg.example.com", "from": "Alerts <alerts@mg.example.com>", "to": "ops@example.com"}
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
		_, _ = w.Write([]byte(`{"id":"<1@mg>","message":"Queued. Thank you."}`))
	}))
	defer srv.Close()
	setup(t, srv.URL)
	if r := send(context.Background(), call(goodCfg(), "key-abc")); !r.Delivered {
		t.Fatalf("got %+v", r)
	}
	if path != "/v3/mg.example.com/messages" || user != "api" || pass != "key-abc" {
		t.Fatalf("path=%s user=%s", path, user)
	}
	if form.Get("subject") != "[FIRING] Disk almost full" || form.Get("to") != "ops@example.com" || !strings.Contains(form.Get("text"), "node-1 at 93%") {
		t.Fatalf("form = %v", form)
	}
}

func TestEURegionUsesEUHost(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.WriteHeader(200)
	}))
	defer srv.Close()
	setup(t, srv.URL)
	cfg := goodCfg()
	cfg["region"] = "eu"
	if r := send(context.Background(), call(cfg, "key-abc")); !r.Delivered || !strings.HasPrefix(path, "/eu/v3/") {
		t.Fatalf("got %+v path=%s", r, path)
	}
}

func TestSendErrorsMapped(t *testing.T) {
	pluginkittest.RunErrorCases(t, func(serverURL string) pluginkit.Result {
		setup(t, serverURL)
		return send(context.Background(), call(goodCfg(), "key-abc"))
	})
}

func TestSendValidation(t *testing.T) {
	setup(t, "http://127.0.0.1:1")
	with := func(k, v string) map[string]string {
		c := goodCfg()
		c[k] = v
		return c
	}
	tests := map[string]pluginkit.Call{
		"bad region":  call(with("region", "asia"), "k"),
		"bad domain":  call(with("domain", "bad domain/x"), "k"),
		"bad from":    call(with("from", "nope"), "k"),
		"bad to":      call(with("to", ""), "k"),
		"no key":      call(goodCfg(), ""),
		"unknown key": {Event: pluginkittest.Event(), Config: json.RawMessage(`{"x":1}`), Secret: "k"},
	}
	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			if r := send(context.Background(), c); r.Delivered || r.Retryable || r.Detail == "" {
				t.Fatalf("got %+v", r)
			}
		})
	}
}
