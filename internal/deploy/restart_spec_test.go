package deploy

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestAppYAMLRestart_CarriesIntoServiceHealth(t *testing.T) {
	tests := []struct {
		restart     string
		wantPolicy  string
		wantOneShot bool
		wantErr     bool
	}{
		{restart: `"no"`, wantPolicy: "no", wantOneShot: true},
		{restart: "on-failure", wantPolicy: "on-failure", wantOneShot: true},
		{restart: "On-Failure:3", wantPolicy: "on-failure:3", wantOneShot: true},
		{restart: "always"},
		{restart: ""},
		{restart: "on-failure:abc", wantErr: true},
	}
	for _, tt := range tests {
		t.Run("restart="+tt.restart, func(t *testing.T) {
			doc := "version: 1\nservices:\n  migrate:\n    build:\n      type: image\n      image: app:v1\n    port: 8080\n"
			if tt.restart != "" {
				doc += "    restart: " + tt.restart + "\n"
			}
			s, err := spec.Parse([]byte(doc))
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "restart") {
					t.Fatalf("Parse err = %v, want a restart validation error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			d, err := toDesiredService("migrate", "app:v1", s.Services["migrate"])
			if err != nil {
				t.Fatalf("toDesiredService: %v", err)
			}
			if d.RunsToCompletion() != tt.wantOneShot {
				t.Fatalf("RunsToCompletion = %v, want %v (Health %+v)", d.RunsToCompletion(), tt.wantOneShot, d.Health)
			}
			if tt.wantOneShot && d.Health.Restart != tt.wantPolicy {
				t.Fatalf("Health.Restart = %q, want %q", d.Health.Restart, tt.wantPolicy)
			}
			if !tt.wantOneShot && d.Health != nil {
				t.Fatalf("Health = %+v, want nil for a keep-running service", d.Health)
			}
		})
	}
}
