package ingress

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInheritSockets(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		mode    string
		wantNil bool
		wantErr string
		https   int
		http    int
	}{
		{name: "no activation", env: map[string]string{}, wantNil: true},
		{name: "other pid", env: map[string]string{"LISTEN_PID": "1", "LISTEN_FDS": "2", "LISTEN_FDNAMES": "https:http"}, wantNil: true},
		{name: "required but absent", env: map[string]string{}, mode: "true", wantErr: "not started with systemd"},
		{name: "disabled", env: map[string]string{"LISTEN_PID": "42", "LISTEN_FDS": "2", "LISTEN_FDNAMES": "https:http"}, mode: "false", wantNil: true},
		{name: "names missing", env: map[string]string{"LISTEN_PID": "42", "LISTEN_FDS": "2"}, wantErr: "FileDescriptorName"},
		{name: "no https socket", env: map[string]string{"LISTEN_PID": "42", "LISTEN_FDS": "1", "LISTEN_FDNAMES": "http"}, mode: "true", wantErr: "no socket named https"},
		{name: "both", env: map[string]string{"LISTEN_PID": "42", "LISTEN_FDS": "2", "LISTEN_FDNAMES": "https:http"}, https: 3, http: 4},
		{name: "reordered", env: map[string]string{"LISTEN_PID": "42", "LISTEN_FDS": "2", "LISTEN_FDNAMES": "http:https"}, https: 4, http: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InheritSockets(func(k string) string { return tt.env[k] }, 42, tt.mode)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantNil {
				if got != nil {
					t.Fatalf("got %+v, want nil", got)
				}
				return
			}
			if got.HTTPS != tt.https || got.HTTP != tt.http {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func TestBuildRoutesConfig_InheritedSockets(t *testing.T) {
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName:   "edge",
		TLS:          true,
		ACMEEnabled:  true,
		ACMEEmail:    "ops@example.com",
		HTTPRedirect: true,
		Routes:       []ProxyRoute{{Hosts: []string{"app.example.com"}, BackendDial: "127.0.0.1:9"}},
		Inherited:    &InheritedSockets{HTTPS: 3, HTTP: 4, HTTPSPort: 443, HTTPPort: 80},
		Hardening:    func() *Hardening { h := DefaultHardening(); return &h }(),
	})
	if err != nil {
		t.Fatal(err)
	}
	main := cfg.Apps.HTTP.Servers["edge"]
	if len(main.Listen) != 1 || main.Listen[0] != "fd/3" {
		t.Errorf("main listen = %v", main.Listen)
	}
	for _, p := range main.Protocols {
		if p == "h3" {
			t.Error("h3 cannot be served from an inherited TCP socket")
		}
	}
	redirect := cfg.Apps.HTTP.Servers["edge-http"]
	if redirect == nil || redirect.Listen[0] != "fd/4" || len(redirect.Routes) != 1 {
		t.Fatalf("redirect server = %+v", redirect)
	}
	raw, _ := json.Marshal(cfg)
	if !strings.Contains(string(raw), `"disabled":true`) {
		t.Error("ACME HTTP-01 must be disabled without a port to attach the challenge to")
	}
	ports := listenPortsOf(cfg)
	if !ports[443] || !ports[80] {
		t.Errorf("owned ports = %v, want 80 and 443 reported to doctor", ports)
	}
}
