package deploy

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
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
	got, err := toServiceResources(spec.Resources{Memory: "512Mi", CPU: 0.5})
	if err != nil {
		t.Fatalf("toServiceResources() error = %v", err)
	}
	if got.MemoryBytes != 512*1024*1024 {
		t.Errorf("MemoryBytes = %d, want %d", got.MemoryBytes, 512*1024*1024)
	}
	if got.NanoCPUs != 500_000_000 {
		t.Errorf("NanoCPUs = %d, want 500000000", got.NanoCPUs)
	}
}

func TestToServiceResources_InvalidMemoryPropagatesError(t *testing.T) {
	_, err := toServiceResources(spec.Resources{Memory: "not-a-size"})
	if err == nil {
		t.Fatal("toServiceResources() error = nil, want an error for an invalid memory string")
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
