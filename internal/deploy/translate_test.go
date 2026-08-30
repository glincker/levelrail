package deploy

import (
	"reflect"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestParseMemoryBytes(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    int64
		wantErr bool
	}{
		{name: "512Mi", in: "512Mi", want: 512 * 1024 * 1024},
		{name: "1Gi", in: "1Gi", want: 1024 * 1024 * 1024},
		{name: "0Mi", in: "0Mi", want: 0},
		{name: "empty", in: "", wantErr: true},
		{name: "no unit", in: "512", wantErr: true},
		{name: "no digits", in: "Mi", wantErr: true},
		{name: "unknown unit", in: "512Ki", wantErr: true},
		{name: "non-numeric prefix", in: "abcMi", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMemoryBytes(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseMemoryBytes(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseMemoryBytes(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestToServiceResources(t *testing.T) {
	got, err := toServiceResources(spec.Resources{Memory: "512Mi", CPU: 0.5, SwapMemory: "1Gi", CPUSet: "0-1"})
	if err != nil {
		t.Fatalf("toServiceResources() error = %v", err)
	}
	if got.MemoryBytes != 512*1024*1024 {
		t.Errorf("MemoryBytes = %d, want %d", got.MemoryBytes, 512*1024*1024)
	}
	if got.NanoCPUs != 500_000_000 {
		t.Errorf("NanoCPUs = %d, want 500000000", got.NanoCPUs)
	}
	if got.SwapMemoryBytes != 1024*1024*1024 {
		t.Errorf("SwapMemoryBytes = %d, want %d", got.SwapMemoryBytes, 1024*1024*1024)
	}
	if got.CPUSetCPUs != "0-1" {
		t.Errorf("CPUSetCPUs = %q, want %q", got.CPUSetCPUs, "0-1")
	}
}

func TestToServiceResources_InvalidMemoryPropagatesError(t *testing.T) {
	_, err := toServiceResources(spec.Resources{Memory: "not-a-size"})
	if err == nil {
		t.Fatal("toServiceResources() error = nil, want an error for an invalid memory string")
	}
}

func TestToServiceResources_InvalidSwapMemoryPropagatesError(t *testing.T) {
	_, err := toServiceResources(spec.Resources{Memory: "512Mi", SwapMemory: "not-a-size"})
	if err == nil {
		t.Fatal("toServiceResources() error = nil, want an error for an invalid swapMemory string")
	}
}

func TestParseDurationOrZero(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    time.Duration
		wantErr bool
	}{
		{name: "empty means unset", in: "", want: 0},
		{name: "seconds", in: "5s", want: 5 * time.Second},
		{name: "minutes", in: "2m", want: 2 * time.Minute},
		{name: "milliseconds", in: "500ms", want: 500 * time.Millisecond},
		{name: "invalid", in: "five seconds", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDurationOrZero(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseDurationOrZero(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseDurationOrZero(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestToServiceProbe(t *testing.T) {
	got, err := toServiceProbe(spec.Probe{Path: "/healthz", Interval: "5s", Timeout: "2s", Failures: 3})
	if err != nil {
		t.Fatalf("toServiceProbe() error = %v", err)
	}
	want := struct {
		Path     string
		Interval time.Duration
		Timeout  time.Duration
		Failures int
	}{"/healthz", 5 * time.Second, 2 * time.Second, 3}
	if got.Path != want.Path || got.Interval != want.Interval || got.Timeout != want.Timeout || got.Failures != want.Failures {
		t.Errorf("toServiceProbe() = %+v, want %+v", got, want)
	}
}

func TestToServiceHealth_NilFieldsStayNil(t *testing.T) {
	got, err := toServiceHealth(spec.Health{})
	if err != nil {
		t.Fatalf("toServiceHealth() error = %v", err)
	}
	if got.Readiness != nil || got.Liveness != nil {
		t.Errorf("toServiceHealth(empty) = %+v, want both nil", got)
	}
}

func TestToServiceHealth_BothProbesTranslated(t *testing.T) {
	got, err := toServiceHealth(spec.Health{
		Readiness: &spec.Probe{Path: "/ready", Interval: "5s"},
		Liveness:  &spec.Probe{Path: "/live", Failures: 3},
	})
	if err != nil {
		t.Fatalf("toServiceHealth() error = %v", err)
	}
	if got.Readiness == nil || got.Readiness.Path != "/ready" {
		t.Errorf("Readiness = %+v, want Path=/ready", got.Readiness)
	}
	if got.Liveness == nil || got.Liveness.Failures != 3 {
		t.Errorf("Liveness = %+v, want Failures=3", got.Liveness)
	}
}

func TestLiteralEnv(t *testing.T) {
	got := literalEnv(map[string]spec.EnvVar{
		"A": {Value: "1"},
		"B": {Value: "2"},
	})
	if got["A"] != "1" || got["B"] != "2" || len(got) != 2 {
		t.Errorf("literalEnv() = %+v, want A=1 B=2", got)
	}
}

func TestLiteralEnv_Empty(t *testing.T) {
	if got := literalEnv(nil); got != nil {
		t.Errorf("literalEnv(nil) = %+v, want nil", got)
	}
}

func TestToDesiredService_FullySpecified(t *testing.T) {
	svc := spec.Service{
		Port: 3000,
		Env:  map[string]spec.EnvVar{"NODE_ENV": {Value: "production"}},
		Resources: &spec.Resources{
			Memory: "512Mi", CPU: 0.5,
		},
		Health: &spec.Health{
			Readiness: &spec.Probe{Path: "/healthz", Interval: "5s"},
		},
	}

	got, err := toDesiredService("web", "img:sha", svc)
	if err != nil {
		t.Fatalf("toDesiredService() error = %v", err)
	}
	if got.Name != "web" || got.Image != "img:sha" || got.Port != 3000 {
		t.Errorf("scalar fields = %+v", got)
	}
	if got.Env["NODE_ENV"] != "production" {
		t.Errorf("Env = %+v", got.Env)
	}
	if got.Resources == nil || got.Resources.MemoryBytes != 512*1024*1024 {
		t.Errorf("Resources = %+v", got.Resources)
	}
	if got.Health == nil || got.Health.Readiness == nil || got.Health.Readiness.Path != "/healthz" {
		t.Errorf("Health = %+v", got.Health)
	}
}

func TestToDesiredService_HostPort_PassesThroughAsPointer(t *testing.T) {
	svc := spec.Service{Port: 8080, HostPort: 30001}
	got, err := toDesiredService("web", "img:sha", svc)
	if err != nil {
		t.Fatalf("toDesiredService() error = %v", err)
	}
	if got.HostPort == nil || *got.HostPort != 30001 {
		t.Errorf("HostPort = %v, want a pointer to 30001", got.HostPort)
	}
}

// TestToDesiredService_NoHostPort_LeavesNil is the regression-safety
// counterpart: an app.yaml with no host_port: (every service before this
// field existed) must produce a nil HostPort, not a pointer to 0.
func TestToDesiredService_NoHostPort_LeavesNil(t *testing.T) {
	svc := spec.Service{Port: 8080}
	got, err := toDesiredService("web", "img:sha", svc)
	if err != nil {
		t.Fatalf("toDesiredService() error = %v", err)
	}
	if got.HostPort != nil {
		t.Errorf("HostPort = %v, want nil", *got.HostPort)
	}
}

func TestToDesiredService_LabelsPassThrough(t *testing.T) {
	svc := spec.Service{Port: 8080, Labels: map[string]string{"team": "platform"}}
	got, err := toDesiredService("web", "img:sha", svc)
	if err != nil {
		t.Fatalf("toDesiredService() error = %v", err)
	}
	if got.Labels["team"] != "platform" || len(got.Labels) != 1 {
		t.Errorf("Labels = %+v, want map[team:platform]", got.Labels)
	}
}

func TestToDesiredService_VolumesGetPlatformPrefixedNames(t *testing.T) {
	svc := spec.Service{Port: 8080, Volumes: []spec.Volume{
		{Name: "data", Path: "/var/lib/data"},
		{Name: "config", Path: "/etc/app"},
	}}
	got, err := toDesiredService("web", "img:sha", svc)
	if err != nil {
		t.Fatalf("toDesiredService() error = %v", err)
	}
	want := []store.ServiceVolume{
		{Name: "app-web-data", ContainerPath: "/var/lib/data"},
		{Name: "app-web-config", ContainerPath: "/etc/app"},
	}
	if !reflect.DeepEqual(got.Volumes, want) {
		t.Errorf("Volumes = %+v, want %+v", got.Volumes, want)
	}
}

func TestToDesiredService_MinimalNoResourcesOrHealth(t *testing.T) {
	got, err := toDesiredService("web", "img:sha", spec.Service{Port: 8080})
	if err != nil {
		t.Fatalf("toDesiredService() error = %v", err)
	}
	if got.Resources != nil {
		t.Errorf("Resources = %+v, want nil", got.Resources)
	}
	if got.Health != nil {
		t.Errorf("Health = %+v, want nil", got.Health)
	}
}

func TestToDesiredService_InvalidResourcesPropagatesError(t *testing.T) {
	svc := spec.Service{Port: 8080, Resources: &spec.Resources{Memory: "not-a-size"}}
	_, err := toDesiredService("web", "img:sha", svc)
	if err == nil {
		t.Fatal("toDesiredService() error = nil, want the resources translation error to propagate")
	}
}

func TestToDesiredService_InvalidHealthPropagatesError(t *testing.T) {
	svc := spec.Service{Port: 8080, Health: &spec.Health{Readiness: &spec.Probe{Path: "/healthz", Interval: "not-a-duration"}}}
	_, err := toDesiredService("web", "img:sha", svc)
	if err == nil {
		t.Fatal("toDesiredService() error = nil, want the health translation error to propagate")
	}
}

func TestParseFromRef(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantDB    string
		wantField string
		wantErr   bool
	}{
		{name: "two segments", in: "main.url", wantDB: "main", wantField: "url"},
		{name: "three segments, engine hint dropped", in: "postgres.main.url", wantDB: "main", wantField: "url"},
		{name: "four segments, only last two matter", in: "cluster.postgres.main.host", wantDB: "main", wantField: "host"},
		{name: "empty", in: "", wantErr: true},
		{name: "single segment", in: "main", wantErr: true},
		{name: "empty database segment", in: ".url", wantErr: true},
		{name: "empty field segment", in: "main.", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDB, gotField, err := parseFromRef(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseFromRef(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if gotDB != tt.wantDB || gotField != tt.wantField {
				t.Errorf("parseFromRef(%q) = (%q, %q), want (%q, %q)", tt.in, gotDB, gotField, tt.wantDB, tt.wantField)
			}
		})
	}
}

func TestDatabaseEnvRefs(t *testing.T) {
	env := map[string]spec.EnvVar{
		"DATABASE_URL": {From: "postgres.main.url"},
		"CACHE_HOST":   {From: "cache.host"},
		"LOG_LEVEL":    {Value: "debug"},
		"API_KEY":      {Secret: true, Required: true},
	}

	got, err := databaseEnvRefs(env)
	if err != nil {
		t.Fatalf("databaseEnvRefs() error = %v", err)
	}
	want := map[string]store.DatabaseEnvRef{
		"DATABASE_URL": {Database: "main", Field: "url"},
		"CACHE_HOST":   {Database: "cache", Field: "host"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("databaseEnvRefs() = %+v, want %+v", got, want)
	}
}

func TestDatabaseEnvRefs_InvalidFrom(t *testing.T) {
	env := map[string]spec.EnvVar{"DATABASE_URL": {From: "main"}}
	if _, err := databaseEnvRefs(env); err == nil {
		t.Fatal("databaseEnvRefs() error = nil, want a single-segment from to be rejected")
	}
}
