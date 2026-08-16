package main

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestBuildAppYAML(t *testing.T) {
	m := mappedApp{
		ServiceName: "web",
		Service: mappedService{
			BuildType: spec.BuildDockerfile,
			BuildPath: "backend/Dockerfile",
			Domains:   []string{"app.example.com"},
			Port:      3000,
			Health:    &mappedHealth{Path: "/healthz", Interval: 5, Timeout: 2, Failures: 3},
			Memory:    "512Mi",
			CPU:       1.5,
			EnvKeys:   []string{"API_KEY", "DATABASE_URL"},
			EnvLiteral: map[string]string{
				"DATABASE_URL": "postgres://real-secret-value",
			},
		},
	}

	data, err := buildAppYAML(m)
	if err != nil {
		t.Fatalf("buildAppYAML() error = %v", err)
	}

	if strings.Contains(string(data), "real-secret-value") {
		t.Fatalf("generated app.yaml leaked a literal secret value:\n%s", data)
	}

	parsed, err := spec.Parse(data)
	if err != nil {
		t.Fatalf("spec.Parse(generated yaml) error = %v\n%s", err, data)
	}

	svc, ok := parsed.Services["web"]
	if !ok {
		t.Fatalf("services map missing %q, got %+v", "web", parsed.Services)
	}
	if svc.Build.Type != spec.BuildDockerfile || svc.Build.Path != "backend/Dockerfile" {
		t.Errorf("Build = %+v, want dockerfile at backend/Dockerfile", svc.Build)
	}
	if svc.Port != 3000 {
		t.Errorf("Port = %d, want 3000", svc.Port)
	}
	if len(svc.Domains) != 1 || svc.Domains[0] != "app.example.com" {
		t.Errorf("Domains = %v, want [app.example.com]", svc.Domains)
	}
	if svc.Health == nil || svc.Health.Readiness == nil || svc.Health.Liveness == nil {
		t.Fatalf("Health = %+v, want both readiness and liveness set", svc.Health)
	}
	if svc.Health.Readiness.Path != "/healthz" || svc.Health.Readiness.Interval != "5s" || svc.Health.Readiness.Timeout != "2s" || svc.Health.Readiness.Failures != 3 {
		t.Errorf("Readiness = %+v, want path /healthz interval 5s timeout 2s failures 3", svc.Health.Readiness)
	}
	if svc.Resources == nil || svc.Resources.Memory != "512Mi" || svc.Resources.CPU != 1.5 {
		t.Errorf("Resources = %+v, want memory 512Mi cpu 1.5", svc.Resources)
	}
	for _, key := range []string{"API_KEY", "DATABASE_URL"} {
		env, ok := svc.Env[key]
		if !ok {
			t.Errorf("Env missing key %q", key)
			continue
		}
		if !env.Secret || env.Value != "" || env.From != "" {
			t.Errorf("Env[%q] = %+v, want a bare secret placeholder", key, env)
		}
	}
}

func TestBuildAppYAML_StaticNoPort(t *testing.T) {
	m := mappedApp{
		ServiceName: "docs",
		Service:     mappedService{BuildType: spec.BuildStatic},
	}
	data, err := buildAppYAML(m)
	if err != nil {
		t.Fatalf("buildAppYAML() error = %v", err)
	}
	if _, err := spec.Parse(data); err != nil {
		t.Fatalf("spec.Parse() error = %v\n%s", err, data)
	}
}

func TestBuildAppYAML_MinimalService(t *testing.T) {
	m := mappedApp{
		ServiceName: "bare",
		Service:     mappedService{BuildType: spec.BuildRailpack, Port: 8080},
	}
	data, err := buildAppYAML(m)
	if err != nil {
		t.Fatalf("buildAppYAML() error = %v", err)
	}
	parsed, err := spec.Parse(data)
	if err != nil {
		t.Fatalf("spec.Parse() error = %v\n%s", err, data)
	}
	if len(parsed.Services) != 1 {
		t.Fatalf("Services = %+v, want exactly one entry", parsed.Services)
	}
}
