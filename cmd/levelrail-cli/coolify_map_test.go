package main

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestMapCoolifyApplication(t *testing.T) {
	tests := []struct {
		name                string
		app                 coolifyApplication
		envs                []coolifyEnvVar
		includeSecretValues bool
		wantBlocking        bool
		wantIssueSubstr     string // substring expected somewhere in Issues, empty means no particular check
		check               func(t *testing.T, got mappedApp)
	}{
		{
			name: "dockerfile build maps directly",
			app: coolifyApplication{
				Name: "My App", BuildPack: "dockerfile", DockerfileLocation: "/backend/Dockerfile",
				FQDN: "https://app.example.com", PortsExposes: "3000",
			},
			check: func(t *testing.T, got mappedApp) {
				if got.Blocking {
					t.Fatalf("Blocking = true, want false")
				}
				if got.ServiceName != "my-app" {
					t.Errorf("ServiceName = %q, want %q", got.ServiceName, "my-app")
				}
				if got.Service.BuildType != spec.BuildDockerfile {
					t.Errorf("BuildType = %q, want %q", got.Service.BuildType, spec.BuildDockerfile)
				}
				if got.Service.BuildPath != "backend/Dockerfile" {
					t.Errorf("BuildPath = %q, want %q", got.Service.BuildPath, "backend/Dockerfile")
				}
				if !reflect.DeepEqual(got.Service.Domains, []string{"app.example.com"}) {
					t.Errorf("Domains = %v, want [app.example.com]", got.Service.Domains)
				}
				if got.Service.Port != 3000 {
					t.Errorf("Port = %d, want 3000", got.Service.Port)
				}
			},
		},
		{
			name: "railpack build maps directly",
			app:  coolifyApplication{Name: "web", BuildPack: "railpack", PortsExposes: "8080"},
			check: func(t *testing.T, got mappedApp) {
				if got.Service.BuildType != spec.BuildRailpack {
					t.Errorf("BuildType = %q, want %q", got.Service.BuildType, spec.BuildRailpack)
				}
			},
		},
		{
			name: "static build drops port",
			app:  coolifyApplication{Name: "docs", BuildPack: "static", PortsExposes: "80"},
			check: func(t *testing.T, got mappedApp) {
				if got.Service.BuildType != spec.BuildStatic {
					t.Errorf("BuildType = %q, want %q", got.Service.BuildType, spec.BuildStatic)
				}
				if got.Service.Port != 0 {
					t.Errorf("Port = %d, want 0 for a static build", got.Service.Port)
				}
				if !hasIssue(got.Issues, "ports_exposes", issueDropped) {
					t.Errorf("Issues = %+v, want a dropped ports_exposes issue", got.Issues)
				}
			},
		},
		{
			name:            "nixpacks build is blocking",
			app:             coolifyApplication{Name: "old-app", BuildPack: "nixpacks"},
			wantBlocking:    true,
			wantIssueSubstr: "Nixpacks",
		},
		{
			name:            "dockercompose build is blocking",
			app:             coolifyApplication{Name: "stack", BuildPack: "dockercompose"},
			wantBlocking:    true,
			wantIssueSubstr: "multi-service",
		},
		{
			name:            "unrecognized build pack is blocking",
			app:             coolifyApplication{Name: "mystery", BuildPack: "something-new"},
			wantBlocking:    true,
			wantIssueSubstr: "unrecognized",
		},
		{
			name: "multiple domains stripped of scheme and trailing slash",
			app: coolifyApplication{
				Name: "multi", BuildPack: "dockerfile", PortsExposes: "3000",
				FQDN: "https://a.example.com/,http://b.example.com",
			},
			check: func(t *testing.T, got mappedApp) {
				want := []string{"a.example.com", "b.example.com"}
				if !reflect.DeepEqual(got.Service.Domains, want) {
					t.Errorf("Domains = %v, want %v", got.Service.Domains, want)
				}
			},
		},
		{
			name: "extra ports are dropped with a note",
			app:  coolifyApplication{Name: "multiport", BuildPack: "dockerfile", PortsExposes: "3000,3001,3002"},
			check: func(t *testing.T, got mappedApp) {
				if got.Service.Port != 3000 {
					t.Errorf("Port = %d, want 3000 (first value)", got.Service.Port)
				}
				if !hasIssue(got.Issues, "ports_exposes", issueDropped) {
					t.Errorf("Issues = %+v, want a dropped ports_exposes issue for the extra ports", got.Issues)
				}
			},
		},
		{
			name: "http health check maps to both readiness and liveness",
			app: coolifyApplication{
				Name: "api", BuildPack: "dockerfile", PortsExposes: "3000",
				HealthCheckEnabled: true, HealthCheckType: "http", HealthCheckPath: "/healthz",
				HealthCheckInterval: 5, HealthCheckTimeout: 2, HealthCheckRetries: 3,
			},
			check: func(t *testing.T, got mappedApp) {
				if got.Service.Health == nil {
					t.Fatalf("Health = nil, want non-nil")
				}
				want := mappedHealth{Path: "/healthz", Interval: 5, Timeout: 2, Failures: 3}
				if *got.Service.Health != want {
					t.Errorf("Health = %+v, want %+v", *got.Service.Health, want)
				}
			},
		},
		{
			name: "cmd health check is dropped",
			app: coolifyApplication{
				Name: "api", BuildPack: "dockerfile", PortsExposes: "3000",
				HealthCheckEnabled: true, HealthCheckType: "cmd", HealthCheckCommand: "curl -f localhost:3000",
			},
			check: func(t *testing.T, got mappedApp) {
				if got.Service.Health != nil {
					t.Errorf("Health = %+v, want nil for a CMD-style check", got.Service.Health)
				}
				if !hasIssue(got.Issues, "health_check", issueDropped) {
					t.Errorf("Issues = %+v, want a dropped health_check issue", got.Issues)
				}
			},
		},
		{
			name: "health check disabled maps nothing",
			app:  coolifyApplication{Name: "api", BuildPack: "dockerfile", PortsExposes: "3000", HealthCheckEnabled: false, HealthCheckPath: "/healthz"},
			check: func(t *testing.T, got mappedApp) {
				if got.Service.Health != nil {
					t.Errorf("Health = %+v, want nil when health_check_enabled is false", got.Service.Health)
				}
			},
		},
		{
			name: "memory in whole gibibytes",
			app:  coolifyApplication{Name: "big", BuildPack: "dockerfile", PortsExposes: "3000", LimitsMemory: "2g"},
			check: func(t *testing.T, got mappedApp) {
				if got.Service.Memory != "2Gi" {
					t.Errorf("Memory = %q, want %q", got.Service.Memory, "2Gi")
				}
			},
		},
		{
			name: "memory in mebibytes",
			app:  coolifyApplication{Name: "small", BuildPack: "dockerfile", PortsExposes: "3000", LimitsMemory: "512m"},
			check: func(t *testing.T, got mappedApp) {
				if got.Service.Memory != "512Mi" {
					t.Errorf("Memory = %q, want %q", got.Service.Memory, "512Mi")
				}
			},
		},
		{
			name: "memory zero is omitted",
			app:  coolifyApplication{Name: "unlimited", BuildPack: "dockerfile", PortsExposes: "3000", LimitsMemory: "0"},
			check: func(t *testing.T, got mappedApp) {
				if got.Service.Memory != "" {
					t.Errorf("Memory = %q, want empty for limits_memory: 0", got.Service.Memory)
				}
			},
		},
		{
			name: "cpu parses to float directly",
			app:  coolifyApplication{Name: "cpubound", BuildPack: "dockerfile", PortsExposes: "3000", LimitsCPUs: "1.5"},
			check: func(t *testing.T, got mappedApp) {
				if got.Service.CPU != 1.5 {
					t.Errorf("CPU = %v, want 1.5", got.Service.CPU)
				}
			},
		},
		{
			name: "cpu zero is omitted",
			app:  coolifyApplication{Name: "unbound", BuildPack: "dockerfile", PortsExposes: "3000", LimitsCPUs: "0"},
			check: func(t *testing.T, got mappedApp) {
				if got.Service.CPU != 0 {
					t.Errorf("CPU = %v, want 0", got.Service.CPU)
				}
			},
		},
		{
			name: "non-positive parsed memory is silently omitted, no issue",
			app:  coolifyApplication{Name: "zeroed", BuildPack: "dockerfile", PortsExposes: "3000", LimitsMemory: "0m"},
			check: func(t *testing.T, got mappedApp) {
				if got.Service.Memory != "" {
					t.Errorf("Memory = %q, want empty for limits_memory: 0m", got.Service.Memory)
				}
				if hasIssue(got.Issues, "limits_memory", issueReview) {
					t.Errorf("Issues = %+v, want no review issue for a validly-parsed non-positive limits_memory", got.Issues)
				}
			},
		},
		{
			name: "non-positive parsed cpu is silently omitted, no issue",
			app:  coolifyApplication{Name: "negcpu", BuildPack: "dockerfile", PortsExposes: "3000", LimitsCPUs: "-1"},
			check: func(t *testing.T, got mappedApp) {
				if got.Service.CPU != 0 {
					t.Errorf("CPU = %v, want 0", got.Service.CPU)
				}
				if hasIssue(got.Issues, "limits_cpus", issueReview) {
					t.Errorf("Issues = %+v, want no review issue for a validly-parsed non-positive limits_cpus", got.Issues)
				}
			},
		},
		{
			name: "env vars default to key-only secret placeholders",
			app:  coolifyApplication{Name: "envtest", BuildPack: "dockerfile", PortsExposes: "3000"},
			envs: []coolifyEnvVar{{Key: "DATABASE_URL", Value: "***", RealValue: ""}, {Key: "API_KEY", Value: "***"}},
			check: func(t *testing.T, got mappedApp) {
				want := []string{"API_KEY", "DATABASE_URL"}
				if !reflect.DeepEqual(got.Service.EnvKeys, want) {
					t.Errorf("EnvKeys = %v, want %v", got.Service.EnvKeys, want)
				}
				if len(got.Service.EnvLiteral) != 0 {
					t.Errorf("EnvLiteral = %v, want empty when --include-secret-values was not passed", got.Service.EnvLiteral)
				}
				if !hasIssue(got.Issues, "environment_variables", issueReview) {
					t.Errorf("Issues = %+v, want a review note about unfetched values", got.Issues)
				}
			},
		},
		{
			name:                "env vars with include-secret-values fetches real values",
			app:                 coolifyApplication{Name: "envtest", BuildPack: "dockerfile", PortsExposes: "3000"},
			envs:                []coolifyEnvVar{{Key: "DATABASE_URL", RealValue: "postgres://real"}, {Key: "PLAIN", Value: "literal-value"}},
			includeSecretValues: true,
			check: func(t *testing.T, got mappedApp) {
				want := map[string]string{"DATABASE_URL": "postgres://real", "PLAIN": "literal-value"}
				if !reflect.DeepEqual(got.Service.EnvLiteral, want) {
					t.Errorf("EnvLiteral = %v, want %v", got.Service.EnvLiteral, want)
				}
			},
		},
		{
			name:                "include-secret-values with no real value returned notes it",
			app:                 coolifyApplication{Name: "envtest", BuildPack: "dockerfile", PortsExposes: "3000"},
			envs:                []coolifyEnvVar{{Key: "REDACTED"}},
			includeSecretValues: true,
			check: func(t *testing.T, got mappedApp) {
				if len(got.Service.EnvLiteral) != 0 {
					t.Errorf("EnvLiteral = %v, want empty", got.Service.EnvLiteral)
				}
				if !hasIssue(got.Issues, "environment_variables", issueReview) {
					t.Errorf("Issues = %+v, want a review note about the missing value", got.Issues)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapCoolifyApplication(tt.app, tt.envs, tt.includeSecretValues)
			if got.Blocking != tt.wantBlocking {
				t.Fatalf("Blocking = %v, want %v (issues=%+v)", got.Blocking, tt.wantBlocking, got.Issues)
			}
			if tt.wantIssueSubstr != "" {
				found := false
				for _, iss := range got.Issues {
					if strings.Contains(iss.Detail, tt.wantIssueSubstr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Issues = %+v, want one containing %q", got.Issues, tt.wantIssueSubstr)
				}
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func hasIssue(issues []migrationIssue, field string, severity issueSeverity) bool {
	for _, iss := range issues {
		if iss.Field == field && iss.Severity == severity {
			return true
		}
	}
	return false
}

func TestSanitizeServiceName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"My App", "my-app"},
		{"already-clean", "already-clean"},
		{"Under_Score & Space!!", "under-score-space"},
		{"123-starts-with-digit", "app-123-starts-with-digit"},
		{"", "app"},
		{"---", "app"},
	}
	for _, tt := range tests {
		if got := sanitizeServiceName(tt.in); got != tt.want {
			t.Errorf("sanitizeServiceName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseDockerMemoryBytes(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"512m", 512 * 1024 * 1024, false},
		{"1g", 1024 * 1024 * 1024, false},
		{"1024", 1024, false},
		{"1.5g", int64(1.5 * 1024 * 1024 * 1024), false},
		{"bogus", 0, true},
		{"5x", 0, true},
	}
	for _, tt := range tests {
		got, err := parseDockerMemoryBytes(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseDockerMemoryBytes(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if err == nil && got != tt.want {
			t.Errorf("parseDockerMemoryBytes(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestFormatMemoryMiGi(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{2 * 1024 * 1024 * 1024, "2Gi"},
		{512 * 1024 * 1024, "512Mi"},
		{1024*1024 + 1, "2Mi"}, // rounds up, never under-reports
	}
	for _, tt := range tests {
		if got := formatMemoryMiGi(tt.in); got != tt.want {
			t.Errorf("formatMemoryMiGi(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMapEnvKeysSorted(t *testing.T) {
	envs := []coolifyEnvVar{{Key: "ZEBRA"}, {Key: "ALPHA"}}
	keys, _, _ := mapEnv(envs, false, nil)
	if !sort.StringsAreSorted(keys) {
		t.Errorf("keys = %v, want sorted", keys)
	}
}
