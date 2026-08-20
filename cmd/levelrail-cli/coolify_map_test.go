package main

import (
	"reflect"
	"sort"
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
		wantMemory          *string
		wantCPU             *float64
		check               func(t *testing.T, got mappedApp)
	}{
		{
			name: "dockerfile build maps directly",
			app: coolifyApplication{
				Name: "My App", BuildPack: "dockerfile", DockerfileLocation: "/backend/Dockerfile",
				FQDN: "https://app.example.com", PortsExposes: "3000",
			},
			check: func(t *testing.T, got mappedApp) {
				assertBasicMapping(t, got, "my-app", spec.BuildDockerfile, "backend/Dockerfile", []string{"app.example.com"}, 3000)
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
				assertHasIssue(t, got.Issues, "ports_exposes", issueDropped, "a dropped ports_exposes issue")
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
				assertHasIssue(t, got.Issues, "ports_exposes", issueDropped, "a dropped ports_exposes issue for the extra ports")
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
				assertHasIssue(t, got.Issues, "health_check", issueDropped, "a dropped health_check issue")
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
			name:       "memory in whole gibibytes",
			app:        coolifyApplication{Name: "big", BuildPack: "dockerfile", PortsExposes: "3000", LimitsMemory: "2g"},
			wantMemory: ptr("2Gi"),
		},
		{
			name:       "memory in mebibytes",
			app:        coolifyApplication{Name: "small", BuildPack: "dockerfile", PortsExposes: "3000", LimitsMemory: "512m"},
			wantMemory: ptr("512Mi"),
		},
		{
			name:       "memory zero is omitted",
			app:        coolifyApplication{Name: "unlimited", BuildPack: "dockerfile", PortsExposes: "3000", LimitsMemory: "0"},
			wantMemory: ptr(""),
		},
		{
			name:    "cpu parses to float directly",
			app:     coolifyApplication{Name: "cpubound", BuildPack: "dockerfile", PortsExposes: "3000", LimitsCPUs: "1.5"},
			wantCPU: ptr(1.5),
		},
		{
			name:    "cpu zero is omitted",
			app:     coolifyApplication{Name: "unbound", BuildPack: "dockerfile", PortsExposes: "3000", LimitsCPUs: "0"},
			wantCPU: ptr(0.0),
		},
		{
			name: "non-positive parsed memory is silently omitted, no issue",
			app:  coolifyApplication{Name: "zeroed", BuildPack: "dockerfile", PortsExposes: "3000", LimitsMemory: "0m"},
			check: func(t *testing.T, got mappedApp) {
				assertMemory(t, got, "")
				assertNoIssue(t, got.Issues, "limits_memory", issueReview, "no review issue for a validly-parsed non-positive limits_memory")
			},
		},
		{
			name: "non-positive parsed cpu is silently omitted, no issue",
			app:  coolifyApplication{Name: "negcpu", BuildPack: "dockerfile", PortsExposes: "3000", LimitsCPUs: "-1"},
			check: func(t *testing.T, got mappedApp) {
				assertCPU(t, got, 0)
				assertNoIssue(t, got.Issues, "limits_cpus", issueReview, "no review issue for a validly-parsed non-positive limits_cpus")
			},
		},
		{
			name: "env vars default to key-only secret placeholders",
			app:  coolifyApplication{Name: "envtest", BuildPack: "dockerfile", PortsExposes: "3000"},
			envs: []coolifyEnvVar{{Key: "DATABASE_URL", Value: "***", RealValue: ""}, {Key: "API_KEY", Value: "***"}},
			check: func(t *testing.T, got mappedApp) {
				assertEnvKeysPlaceholder(t, got, []string{"API_KEY", "DATABASE_URL"}, "environment_variables")
			},
		},
		{
			name:                "env vars with include-secret-values fetches real values",
			app:                 coolifyApplication{Name: "envtest", BuildPack: "dockerfile", PortsExposes: "3000"},
			envs:                []coolifyEnvVar{{Key: "DATABASE_URL", RealValue: "postgres://real"}, {Key: "PLAIN", Value: "literal-value"}},
			includeSecretValues: true,
			check: func(t *testing.T, got mappedApp) {
				assertEnvLiteral(t, got, map[string]string{"DATABASE_URL": "postgres://real", "PLAIN": "literal-value"})
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
				assertHasIssue(t, got.Issues, "environment_variables", issueReview, "a review note about the missing value")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapCoolifyApplication(tt.app, tt.envs, tt.includeSecretValues)
			runMapTestCase(t, tt.wantBlocking, tt.wantIssueSubstr, tt.check, got)
			if tt.wantMemory != nil {
				assertMemory(t, got, *tt.wantMemory)
			}
			if tt.wantCPU != nil {
				assertCPU(t, got, *tt.wantCPU)
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
