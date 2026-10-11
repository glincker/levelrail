package compose

import (
	"slices"
	"strings"
	"testing"
)

func TestValidate_RejectsHostAccess(t *testing.T) {
	tests := []struct {
		name    string
		service string
		wantErr string
	}{
		{"privileged", "privileged: true", "privileged: true is not allowed [privileged]"},
		{"host network", "network_mode: host", "[network_host]"},
		{"container network", "network_mode: \"service:db\"", "network_mode: \"service:db\" is not supported"},
		{"host pid", "pid: host", "pid: host shares a host namespace"},
		{"host ipc", "ipc: host", "[host_ipc]"},
		{"host userns", "userns_mode: host", "[host_userns]"},
		{"cap_add", "cap_add: [SYS_ADMIN]", "cap_add: SYS_ADMIN is not allowed"},
		{"devices", "devices: [\"/dev/kvm:/dev/kvm\"]", "devices: is not allowed"},
		{"seccomp off", "security_opt: [\"seccomp=unconfined\"]", "[security_opt_weakened]"},
		{"volumes_from", "volumes_from: [other]", "[volumes_from]"},
		{"docker socket bind", "volumes: [\"/var/run/docker.sock:/var/run/docker.sock\"]", "Docker socket"},
		{"docker socket elsewhere", "volumes: [\"/home/me/docker.sock:/docker.sock\"]", "Docker socket"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := "services:\n  app:\n    image: nginx:alpine\n    " + tt.service + "\n"
			f, err := Parse([]byte(body))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			err = f.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_AllowsHarmlessHostAccessValues(t *testing.T) {
	body := `services:
  app:
    image: nginx:alpine
    network_mode: bridge
    security_opt: ["no-new-privileges:true"]
`
	f, err := Parse([]byte(body))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestParse_DependsOnConditions(t *testing.T) {
	tests := []struct {
		name        string
		dependsOn   string
		wantHealthy []string
		wantErr     string
	}{
		{"list form has no conditions", "[db, cache]", nil, ""},
		{"long form healthy", "{db: {condition: service_healthy}, cache: {condition: service_started}}", []string{"db"}, ""},
		{"completed treated as started", "{db: {condition: service_completed_successfully}, cache: {}}", nil, ""},
		{"unknown condition", "{db: {condition: service_ready}, cache: {}}", nil, "unknown condition"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := "services:\n  db: {image: postgres:16}\n  cache: {image: redis:7}\n  web:\n    image: nginx:alpine\n    depends_on: " + tt.dependsOn + "\n"
			f, err := Parse([]byte(body))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Parse() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if got := f.Services["web"].DependsOnHealthy; !slices.Equal(got, tt.wantHealthy) {
				t.Errorf("DependsOnHealthy = %v, want %v", got, tt.wantHealthy)
			}
			svcs, _, err := ToDesiredServices("stack", f)
			if err != nil {
				t.Fatalf("ToDesiredServices() error = %v", err)
			}
			for _, s := range svcs {
				if s.Name == "stack-web" && !slices.Equal(s.DependsOnHealthy, tt.wantHealthy) {
					t.Errorf("translated DependsOnHealthy = %v, want %v", s.DependsOnHealthy, tt.wantHealthy)
				}
			}
		})
	}
}

func TestApplyProfiles(t *testing.T) {
	body := `services:
  web: {image: nginx:alpine}
  debug: {image: busybox, profiles: [debug]}
  worker: {image: busybox, profiles: [jobs, debug]}
x-levelrail-domains:
  debug: debug.example.com
`
	tests := []struct {
		name        string
		active      []string
		wantKept    []string
		wantDropped []string
	}{
		{"no profiles", nil, []string{"web"}, []string{"debug", "worker"}},
		{"debug", []string{"debug"}, []string{"debug", "web", "worker"}, nil},
		{"jobs", []string{"jobs"}, []string{"web", "worker"}, []string{"debug"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse([]byte(body))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if got := f.Profiles(); !slices.Equal(got, []string{"debug", "jobs"}) {
				t.Errorf("Profiles() = %v", got)
			}
			dropped, err := f.ApplyProfiles(tt.active)
			if err != nil {
				t.Fatalf("ApplyProfiles() error = %v", err)
			}
			if !slices.Equal(dropped, tt.wantDropped) {
				t.Errorf("dropped = %v, want %v", dropped, tt.wantDropped)
			}
			if got := sortedServiceNames(f); !slices.Equal(got, tt.wantKept) {
				t.Errorf("kept = %v, want %v", got, tt.wantKept)
			}
			if _, ok := f.Domains["debug"]; ok == slices.Contains(tt.wantDropped, "debug") {
				t.Errorf("domain for debug kept = %v, want dropped with the service", ok)
			}
		})
	}
}

func TestApplyProfiles_DependencyOnDroppedService(t *testing.T) {
	f, err := Parse([]byte("services:\n  web: {image: nginx:alpine, depends_on: [db]}\n  db: {image: postgres:16, profiles: [db]}\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if _, err := f.ApplyProfiles(nil); err == nil || !strings.Contains(err.Error(), "depends on \"db\"") {
		t.Fatalf("ApplyProfiles() error = %v, want a dependency error", err)
	}
}
