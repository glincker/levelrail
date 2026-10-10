package ingress

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildRoutesConfig_HiddenDomain(t *testing.T) {
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "ingress",
		ListenAddr: ":443",
		Routes: []ProxyRoute{
			{Hosts: []string{"console.example.internal"}, BackendDial: "127.0.0.1:9001", Hidden: true},
			{Hosts: []string{"open.example.internal"}, BackendDial: "127.0.0.1:9002"},
		},
	})
	if err != nil {
		t.Fatalf("BuildRoutesConfig() error: %v", err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	got := string(raw)
	for _, want := range []string{
		`"path":["/robots.txt"]`,
		`Disallow: /`,
		`"handler":"headers"`,
		`"X-Robots-Tag":["noindex, nofollow, noarchive, nosnippet"]`,
		`"deferred":true`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("config missing %s", want)
		}
	}
	if n := strings.Count(got, `"path":["/robots.txt"]`); n != 1 {
		t.Errorf("robots.txt routes = %d, want 1: an open domain must not get one", n)
	}

	open := decodeRoutesByHost(t, cfg)["open.example.internal"]
	if len(open) != 1 || open[0].(map[string]any)["handler"] != "reverse_proxy" {
		t.Errorf("open route handle = %v, want exactly [reverse_proxy]", open)
	}
	hidden := decodeRoutesByHost(t, cfg)["console.example.internal"]
	if len(hidden) != 2 || hidden[0].(map[string]any)["handler"] != "headers" || hidden[1].(map[string]any)["handler"] != "reverse_proxy" {
		t.Errorf("hidden route handle = %v, want [headers, reverse_proxy]", hidden)
	}
}
