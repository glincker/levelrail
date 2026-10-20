package main

import (
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/localmode"
)

func TestParseDevArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    devOptions
		wantErr bool
	}{
		{"default start", nil, devOptions{}, false},
		{"flags", []string{"--port", "9000", "--no-open", "--reset"}, devOptions{port: 9000, noOpen: true, reset: true}, false},
		{"stop", []string{"stop"}, devOptions{sub: "stop"}, false},
		{"status json", []string{"status", "--json"}, devOptions{sub: "status", jsonOut: true}, false},
		{"unknown sub", []string{"restart"}, devOptions{}, true},
		{"bad port", []string{"--port", "70000"}, devOptions{}, true},
		{"extra arg", []string{"--no-open", "oops"}, devOptions{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseDevArgs("levelrail-cli", tc.args, io.Discard)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestDefaultDevDataDir(t *testing.T) {
	env := func(m map[string]string) func(string) (string, bool) {
		return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
	}
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"explicit", map[string]string{envDevDataDir: "/srv/dev"}, "/srv/dev"},
		{"xdg", map[string]string{"XDG_DATA_HOME": "/x", "HOME": "/h"}, "/x/levelrail-dev"},
		{"home", map[string]string{"HOME": "/h"}, "/h/.local/share/levelrail-dev"},
	}
	for _, tc := range tests {
		got, err := defaultDevDataDir("levelrail-cli", env(tc.env))
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
}

func TestDefaultDevDataDirIgnoresServerDataDir(t *testing.T) {
	got, err := defaultDevDataDir("levelrail-cli", func(k string) (string, bool) {
		if k == "APP_DATA_DIR" {
			return "/var/lib/prod-data", true
		}
		if k == "HOME" {
			return "/h", true
		}
		return "", false
	})
	if err != nil || strings.Contains(got, "prod-data") {
		t.Fatalf("a server data dir leaked into dev mode: %q, %v", got, err)
	}
}

func TestControlPlaneBinary(t *testing.T) {
	none := func(string) (string, error) { return "", exec.ErrNotFound }
	noEnv := func(string) (string, bool) { return "", false }
	exists := func(set ...string) func(string) bool {
		return func(p string) bool {
			for _, s := range set {
				if s == p {
					return true
				}
			}
			return false
		}
	}
	tests := []struct {
		name    string
		flag    string
		self    string
		exists  func(string) bool
		look    func(string) (string, error)
		want    string
		wantErr bool
	}{
		{"flag", "/opt/cp", "/usr/bin/levelrail-cli", exists("/opt/cp"), none, "/opt/cp", false},
		{"flag missing", "/opt/cp", "", exists(), none, "", true},
		{"sibling", "", "/usr/local/bin/levelrail-cli", exists("/usr/local/bin/levelrail"), none, "/usr/local/bin/levelrail", false},
		{"path", "", "/tmp/levelrail-cli", exists(), func(string) (string, error) { return "/bin/levelrail", nil }, "/bin/levelrail", false},
		{"nowhere", "", "/tmp/levelrail-cli", exists(), none, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := controlPlaneBinary("levelrail-cli", tc.flag, noEnv, tc.self, tc.exists, tc.look)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("got %q, %v; want %q, wantErr %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestDevChildEnv(t *testing.T) {
	base := []string{"PATH=/bin", "APP_DEV_MODE=1", "APP_DATA_DIR=/var/lib/prod", "APP_PUBLIC_HOST=1.2.3.4", "APP_HTTP_ADDR=:80", "APP_ADMIN_PASSWORD=old"}
	got := devChildEnv(base, devEnvConfig{
		DataDir: "/d", Ports: localmode.Ports{Dashboard: 8080, HTTP: 8081, HTTPS: 8443, Agent: 9443},
		Admin: localmode.Admin{Username: "admin", Password: "pw"}, DockerSock: "/s.sock",
	})
	env := map[string]string{}
	for _, kv := range got {
		k, v, _ := strings.Cut(kv, "=")
		if _, dup := env[k]; dup {
			t.Errorf("duplicate key %s", k)
		}
		env[k] = v
	}
	want := map[string]string{
		"PATH": "/bin", "APP_LOCAL_MODE": "1", "APP_DATA_DIR": "/d", "APP_HTTP_ADDR": "127.0.0.1:8080",
		"APP_INGRESS_HTTP_ADDR": "127.0.0.1:8081", "APP_INGRESS_HTTPS_ADDR": "127.0.0.1:8443", "APP_AGENT_ADDR": "127.0.0.1:9443",
		"APP_PUBLIC_HOST": "localhost", "APP_ADMIN_PASSWORD": "pw", "APP_CONTAINER_RUNTIME_SOCKET": "unix:///s.sock",
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s = %q, want %q", k, env[k], v)
		}
	}
	if _, ok := env["APP_DEV_MODE"]; ok {
		t.Error("APP_DEV_MODE leaked into the child env")
	}
}
