package spec

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func specWithHealth(health string) []byte {
	return []byte(`version: 1
services:
  web:
    build: { type: dockerfile, path: ./Dockerfile }
    port: 8443
    health:
` + health)
}

func TestParse_ProbeFields(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name    string
		health  string
		want    Probe
		wantErr string
	}{
		{
			name:   "pre-existing spec is unchanged",
			health: "      readiness: { path: /healthz, interval: 5s, timeout: 2s }\n",
			want:   Probe{Path: "/healthz", Interval: "5s", Timeout: "2s"},
		},
		{
			name:   "https with every HTTP option",
			health: "      readiness: { path: /health, scheme: https, host: app.example.com, tls_skip_verify: true, follow_redirects: true, expected_status: 200-399 }\n",
			want:   Probe{Path: "/health", Scheme: "https", Host: "app.example.com", TLSSkipVerify: true, FollowRedirects: &yes, ExpectedStatus: "200-399"},
		},
		{
			name:   "expected_status as a list of codes and ranges",
			health: "      readiness: { path: /, follow_redirects: false, expected_status: [200, 301-302] }\n",
			want:   Probe{Path: "/", FollowRedirects: &no, ExpectedStatus: "200,301-302"},
		},
		{
			name:   "expected_status as a single integer",
			health: "      readiness: { path: /, expected_status: 204 }\n",
			want:   Probe{Path: "/", ExpectedStatus: "204"},
		},
		{
			name:   "exec as a shell string",
			health: "      readiness: { exec: \"pg_isready -U app\", timeout: 5s }\n",
			want:   Probe{Exec: ExecCommand{"/bin/sh", "-c", "pg_isready -U app"}, Timeout: "5s"},
		},
		{
			name:   "exec as argv",
			health: "      readiness: { exec: [redis-cli, ping] }\n",
			want:   Probe{Exec: ExecCommand{"redis-cli", "ping"}},
		},
		{name: "neither path nor exec", health: "      readiness: { interval: 5s }\n", wantErr: "schema validation"},
		{name: "unknown key rejected", health: "      readiness: { path: /, follow_redirect: true }\n", wantErr: "schema validation"},
		{name: "bad scheme", health: "      readiness: { path: /, scheme: ftp }\n", wantErr: "schema validation"},
		{name: "path and exec together", health: "      readiness: { path: /, exec: [\"true\"] }\n", wantErr: `service "web": health.readiness: set either path (HTTP probe) or exec (command probe), not both`},
		{name: "tls_skip_verify needs https", health: "      liveness: { path: /, tls_skip_verify: true }\n", wantErr: `service "web": health.liveness: tls_skip_verify only applies to scheme: https`},
		{name: "backwards status range", health: "      readiness: { path: /, expected_status: 399-200 }\n", wantErr: `range "399-200" runs backwards`},
		{name: "HTTP-only field on exec", health: "      readiness: { exec: [\"true\"], expected_status: 200 }\n", wantErr: "apply to HTTP probes only"},
		{name: "relative path", health: "      readiness: { path: healthz }\n", wantErr: `path "healthz" must start with /`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Parse(specWithHealth(tt.health))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Parse() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			got := s.Services["web"].Health.Readiness
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("readiness = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestProbe_JSONDecoding(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Probe
	}{
		{name: "string status and shell exec", in: `{"exec":"redis-cli ping","expected_status":"200-399"}`, want: Probe{Exec: ExecCommand{"/bin/sh", "-c", "redis-cli ping"}, ExpectedStatus: "200-399"}},
		{name: "numeric status list and argv", in: `{"path":"/","expected_status":[200,"301-302"],"tls_skip_verify":true,"scheme":"https"}`, want: Probe{Path: "/", ExpectedStatus: "200,301-302", TLSSkipVerify: true, Scheme: "https"}},
		{name: "single numeric status", in: `{"path":"/","expected_status":204}`, want: Probe{Path: "/", ExpectedStatus: "204"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Probe
			if err := json.Unmarshal([]byte(tt.in), &got); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
