package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestPlanFromFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    createFlags
		fileSpec *spec.Spec
		detected detectedGit
		wantErr  string // substring; empty means no error
		wantPlan func(t *testing.T, p createPlan)
	}{
		{
			name:    "no mode selected",
			flags:   createFlags{},
			wantErr: "specify one of",
		},
		{
			name:    "image and repo mutually exclusive",
			flags:   createFlags{image: "x:1", repo: "https://example.com/x.git"},
			wantErr: "mutually exclusive",
		},
		{
			name:    "existing image missing name",
			flags:   createFlags{image: "x:1", port: 3000},
			wantErr: "--name",
		},
		{
			name:    "existing image missing port",
			flags:   createFlags{image: "x:1", name: "web"},
			wantErr: "--port",
		},
		{
			name:  "existing image success",
			flags: createFlags{image: "registry.example.com/x:1", name: "web", port: 3000},
			wantPlan: func(t *testing.T, p createPlan) {
				if p.Build != nil {
					t.Errorf("Build = %+v, want nil for existing-image path", p.Build)
				}
				want := appResource{Name: "web", Image: "registry.example.com/x:1", Port: 3000}
				if !reflect.DeepEqual(p.CreateBody, want) {
					t.Errorf("CreateBody = %+v, want %+v", p.CreateBody, want)
				}
			},
		},
		{
			name:    "git-build flags missing image-repo",
			flags:   createFlags{repo: "https://example.com/x.git", name: "web", port: 3000},
			wantErr: "--image-repo",
		},
		{
			name:  "git-build flags success",
			flags: createFlags{repo: "https://example.com/x.git", name: "web", port: 3000, imageRepo: "levelrail/web", ref: "release"},
			wantPlan: func(t *testing.T, p createPlan) {
				if p.Build == nil {
					t.Fatalf("Build = nil, want non-nil for git-build path")
				}
				wantBuild := buildTriggerRequest{RepoURL: "https://example.com/x.git", Ref: "release", ImageRepo: "levelrail/web"}
				if *p.Build != wantBuild {
					t.Errorf("Build = %+v, want %+v", *p.Build, wantBuild)
				}
				if p.CreateBody.Image != "levelrail/web:pending" {
					t.Errorf("CreateBody.Image = %q, want placeholder %q", p.CreateBody.Image, "levelrail/web:pending")
				}
			},
		},
		{
			name:     "git-build flags ref defaults to detected branch",
			flags:    createFlags{repo: "https://example.com/x.git", name: "web", port: 3000, imageRepo: "levelrail/web"},
			detected: detectedGit{Ref: "feature-branch"},
			wantPlan: func(t *testing.T, p createPlan) {
				if p.Build.Ref != "feature-branch" {
					t.Errorf("Build.Ref = %q, want %q", p.Build.Ref, "feature-branch")
				}
			},
		},
		{
			name:  "git-build flags ref defaults to main with nothing detected",
			flags: createFlags{repo: "https://example.com/x.git", name: "web", port: 3000, imageRepo: "levelrail/web"},
			wantPlan: func(t *testing.T, p createPlan) {
				if p.Build.Ref != "main" {
					t.Errorf("Build.Ref = %q, want %q", p.Build.Ref, "main")
				}
			},
		},
		{
			name:     "git-build flags repo auto-detected",
			flags:    createFlags{name: "web", port: 3000, imageRepo: "levelrail/web"},
			detected: detectedGit{RepoURL: "https://example.com/detected.git", Ref: "main"},
			wantPlan: func(t *testing.T, p createPlan) {
				if p.Build == nil {
					t.Fatalf("Build = nil, want non-nil when a repo is auto-detected")
				}
				if p.Build.RepoURL != "https://example.com/detected.git" {
					t.Errorf("Build.RepoURL = %q, want auto-detected repo", p.Build.RepoURL)
				}
			},
		},
		{
			name:    "file mode with image is rejected",
			flags:   createFlags{file: "app.yaml", image: "x:1"},
			wantErr: "cannot be combined with --file",
		},
		{
			name:  "file mode single service, minimal",
			flags: createFlags{file: "app.yaml", imageRepo: "levelrail/web"},
			fileSpec: &spec.Spec{Services: map[string]spec.Service{
				"web": {Build: spec.Build{Type: spec.BuildDockerfile}, Port: 3000},
			}},
			detected: detectedGit{RepoURL: "https://example.com/detected.git", Ref: "main"},
			wantPlan: func(t *testing.T, p createPlan) {
				if p.CreateBody.Name != "web" {
					t.Errorf("Name = %q, want service key %q", p.CreateBody.Name, "web")
				}
				if p.CreateBody.Port != 3000 {
					t.Errorf("Port = %d, want 3000", p.CreateBody.Port)
				}
				if p.Build.RepoURL != "https://example.com/detected.git" {
					t.Errorf("Build.RepoURL = %q, want auto-detected repo", p.Build.RepoURL)
				}
			},
		},
		{
			name:  "file mode multiple services requires --service",
			flags: createFlags{file: "app.yaml", imageRepo: "levelrail/web", repo: "https://example.com/x.git"},
			fileSpec: &spec.Spec{Services: map[string]spec.Service{
				"web":    {Build: spec.Build{Type: spec.BuildDockerfile}, Port: 3000},
				"worker": {Build: spec.Build{Type: spec.BuildDockerfile}, Port: 4000},
			}},
			wantErr: "declares 2 services",
		},
		{
			name:  "file mode --service selects among multiple",
			flags: createFlags{file: "app.yaml", imageRepo: "levelrail/web", repo: "https://example.com/x.git", service: "worker"},
			fileSpec: &spec.Spec{Services: map[string]spec.Service{
				"web":    {Build: spec.Build{Type: spec.BuildDockerfile}, Port: 3000},
				"worker": {Build: spec.Build{Type: spec.BuildDockerfile}, Port: 4000},
			}},
			wantPlan: func(t *testing.T, p createPlan) {
				if p.CreateBody.Name != "worker" || p.CreateBody.Port != 4000 {
					t.Errorf("CreateBody = %+v, want name=worker port=4000", p.CreateBody)
				}
			},
		},
		{
			name:  "file mode unknown --service",
			flags: createFlags{file: "app.yaml", imageRepo: "levelrail/web", repo: "https://example.com/x.git", service: "ghost"},
			fileSpec: &spec.Spec{Services: map[string]spec.Service{
				"web": {Build: spec.Build{Type: spec.BuildDockerfile}, Port: 3000},
			}},
			wantErr: "not found",
		},
		{
			name:  "file mode non-dockerfile build type rejected",
			flags: createFlags{file: "app.yaml", imageRepo: "levelrail/web", repo: "https://example.com/x.git"},
			fileSpec: &spec.Spec{Services: map[string]spec.Service{
				"web": {Build: spec.Build{Type: spec.BuildStatic}, Port: 3000},
			}},
			wantErr: "only supports",
		},
		{
			name:  "file mode secret env vars rejected",
			flags: createFlags{file: "app.yaml", imageRepo: "levelrail/web", repo: "https://example.com/x.git"},
			fileSpec: &spec.Spec{Services: map[string]spec.Service{
				"web": {
					Build: spec.Build{Type: spec.BuildDockerfile}, Port: 3000,
					Env: map[string]spec.EnvVar{"API_KEY": {Secret: true}},
				},
			}},
			wantErr: "secret env var",
		},
		{
			name:  "file mode missing port and no --port flag",
			flags: createFlags{file: "app.yaml", imageRepo: "levelrail/web", repo: "https://example.com/x.git"},
			fileSpec: &spec.Spec{Services: map[string]spec.Service{
				"web": {Build: spec.Build{Type: spec.BuildDockerfile}},
			}},
			wantErr: "no port set",
		},
		{
			name:  "file mode --port overrides spec",
			flags: createFlags{file: "app.yaml", imageRepo: "levelrail/web", repo: "https://example.com/x.git", port: 9000},
			fileSpec: &spec.Spec{Services: map[string]spec.Service{
				"web": {Build: spec.Build{Type: spec.BuildDockerfile}, Port: 3000},
			}},
			wantPlan: func(t *testing.T, p createPlan) {
				if p.CreateBody.Port != 9000 {
					t.Errorf("Port = %d, want overridden 9000", p.CreateBody.Port)
				}
			},
		},
		{
			name:  "file mode missing image-repo",
			flags: createFlags{file: "app.yaml", repo: "https://example.com/x.git"},
			fileSpec: &spec.Spec{Services: map[string]spec.Service{
				"web": {Build: spec.Build{Type: spec.BuildDockerfile}, Port: 3000},
			}},
			wantErr: "--image-repo",
		},
		{
			name:  "file mode missing repo with nothing detected",
			flags: createFlags{file: "app.yaml", imageRepo: "levelrail/web"},
			fileSpec: &spec.Spec{Services: map[string]spec.Service{
				"web": {Build: spec.Build{Type: spec.BuildDockerfile}, Port: 3000},
			}},
			wantErr: "--repo is required",
		},
		{
			name:  "file mode domains and env pass through",
			flags: createFlags{file: "app.yaml", imageRepo: "levelrail/web", repo: "https://example.com/x.git"},
			fileSpec: &spec.Spec{Services: map[string]spec.Service{
				"web": {
					Build:   spec.Build{Type: spec.BuildDockerfile},
					Port:    3000,
					Domains: []string{"app.example.com"},
					Env:     map[string]spec.EnvVar{"FOO": {Value: "bar"}},
				},
			}},
			wantPlan: func(t *testing.T, p createPlan) {
				if len(p.CreateBody.Domains) != 1 || p.CreateBody.Domains[0] != "app.example.com" {
					t.Errorf("Domains = %v, want [app.example.com]", p.CreateBody.Domains)
				}
				if p.CreateBody.Env["FOO"] != "bar" {
					t.Errorf("Env[FOO] = %q, want %q", p.CreateBody.Env["FOO"], "bar")
				}
			},
		},
		{
			name:  "file mode resources and health translated",
			flags: createFlags{file: "app.yaml", imageRepo: "levelrail/web", repo: "https://example.com/x.git"},
			fileSpec: &spec.Spec{Services: map[string]spec.Service{
				"web": {
					Build:     spec.Build{Type: spec.BuildDockerfile},
					Port:      3000,
					Resources: &spec.Resources{Memory: "512Mi", CPU: 0.5},
					Health:    &spec.Health{Readiness: &spec.Probe{Path: "/healthz", Interval: "5s", Timeout: "2s", Failures: 3}},
				},
			}},
			wantPlan: func(t *testing.T, p createPlan) {
				if p.CreateBody.Resources == nil {
					t.Fatalf("Resources = nil, want non-nil")
				}
				if p.CreateBody.Resources.MemoryBytes != 512*1024*1024 {
					t.Errorf("MemoryBytes = %d, want %d", p.CreateBody.Resources.MemoryBytes, 512*1024*1024)
				}
				if p.CreateBody.Resources.NanoCPUs != 500_000_000 {
					t.Errorf("NanoCPUs = %d, want 500000000", p.CreateBody.Resources.NanoCPUs)
				}
				if p.CreateBody.Health == nil || p.CreateBody.Health.Readiness == nil {
					t.Fatalf("Health.Readiness = nil, want non-nil")
				}
				r := p.CreateBody.Health.Readiness
				if r.Path != "/healthz" || r.Interval != int64(5*1e9) || r.Timeout != int64(2*1e9) || r.Failures != 3 {
					t.Errorf("Readiness probe = %+v, want path=/healthz interval=5s timeout=2s failures=3", r)
				}
			},
		},
		{
			name:  "file mode dockerfile path from spec",
			flags: createFlags{file: "app.yaml", imageRepo: "levelrail/web", repo: "https://example.com/x.git"},
			fileSpec: &spec.Spec{Services: map[string]spec.Service{
				"web": {Build: spec.Build{Type: spec.BuildDockerfile, Path: "docker/Dockerfile"}, Port: 3000},
			}},
			wantPlan: func(t *testing.T, p createPlan) {
				if p.Build.Build.Path != "docker/Dockerfile" {
					t.Errorf("Build.Path = %q, want %q", p.Build.Build.Path, "docker/Dockerfile")
				}
			},
		},
		{
			name:  "file mode --dockerfile overrides spec",
			flags: createFlags{file: "app.yaml", imageRepo: "levelrail/web", repo: "https://example.com/x.git", dockerfile: "Dockerfile.prod"},
			fileSpec: &spec.Spec{Services: map[string]spec.Service{
				"web": {Build: spec.Build{Type: spec.BuildDockerfile, Path: "docker/Dockerfile"}, Port: 3000},
			}},
			wantPlan: func(t *testing.T, p createPlan) {
				if p.Build.Build.Path != "Dockerfile.prod" {
					t.Errorf("Build.Path = %q, want overridden %q", p.Build.Build.Path, "Dockerfile.prod")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := planFromFlags(tt.flags, tt.fileSpec, tt.detected)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("err = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
				if _, ok := err.(*validationError); !ok {
					t.Errorf("err type = %T, want *validationError", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if tt.wantPlan != nil {
				tt.wantPlan(t, plan)
			}
		})
	}
}

func TestParseMemoryBytes(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{in: "512Mi", want: 512 * 1024 * 1024},
		{in: "1Gi", want: 1024 * 1024 * 1024},
		{in: "0Mi", want: 0},
		{in: "", wantErr: true},
		{in: "512", wantErr: true},
		{in: "Mi", wantErr: true},
		{in: "512Ki", wantErr: true},
		{in: "512mi", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseMemoryBytes(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseMemoryBytes(%q) err = nil, want error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMemoryBytes(%q) err = %v, want nil", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("parseMemoryBytes(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveRef(t *testing.T) {
	tests := []struct {
		name        string
		flagRef     string
		detectedRef string
		want        string
	}{
		{name: "flag wins", flagRef: "release", detectedRef: "feature", want: "release"},
		{name: "detected used when flag empty", flagRef: "", detectedRef: "feature", want: "feature"},
		{name: "main when both empty", flagRef: "", detectedRef: "", want: "main"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveRef(tt.flagRef, tt.detectedRef); got != tt.want {
				t.Errorf("resolveRef(%q, %q) = %q, want %q", tt.flagRef, tt.detectedRef, got, tt.want)
			}
		})
	}
}

func TestPendingImageTag(t *testing.T) {
	if got, want := pendingImageTag("levelrail/web"), "levelrail/web:pending"; got != want {
		t.Errorf("pendingImageTag() = %q, want %q", got, want)
	}
}
