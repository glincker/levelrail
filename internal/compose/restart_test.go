package compose

import "testing"

func TestToDesiredServices_RestartPolicy(t *testing.T) {
	tests := []struct {
		restart     string
		wantPolicy  string
		wantOneShot bool
	}{
		{restart: `"no"`, wantPolicy: "no", wantOneShot: true},
		{restart: "on-failure", wantPolicy: "on-failure", wantOneShot: true},
		{restart: `"on-failure:3"`, wantPolicy: "on-failure:3", wantOneShot: true},
		{restart: "always"},
		{restart: "unless-stopped"},
		{restart: ""},
	}
	for _, tt := range tests {
		t.Run("restart="+tt.restart, func(t *testing.T) {
			yaml := "services:\n  migrate:\n    image: app:v1\n"
			if tt.restart != "" {
				yaml += "    restart: " + tt.restart + "\n"
			}
			f, err := Parse([]byte(yaml))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			svcs, _, err := ToDesiredServices("app", f)
			if err != nil {
				t.Fatalf("ToDesiredServices: %v", err)
			}
			got := svcs[0]
			if got.RunsToCompletion() != tt.wantOneShot {
				t.Fatalf("RunsToCompletion = %v, want %v", got.RunsToCompletion(), tt.wantOneShot)
			}
			if !tt.wantOneShot {
				// Unchanged from before the field existed, so an upgrade sees no drift.
				if got.Health != nil {
					t.Fatalf("Health = %+v, want nil for a keep-running service", got.Health)
				}
				return
			}
			if got.Health.Restart != tt.wantPolicy {
				t.Fatalf("Health.Restart = %q, want %q", got.Health.Restart, tt.wantPolicy)
			}
		})
	}
}
