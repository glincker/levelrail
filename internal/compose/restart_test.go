package compose

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

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

func TestParse_DependsOnCompletedSuccessfully(t *testing.T) {
	f, err := Parse([]byte(`
services:
  web:
    image: app:v1
    depends_on:
      migrate:
        condition: service_completed_successfully
      db:
        condition: service_healthy
  migrate:
    image: app:v1
    restart: "no"
  db:
    image: postgres:16
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	svcs, _, err := ToDesiredServices("app", f)
	if err != nil {
		t.Fatalf("ToDesiredServices: %v", err)
	}
	var web *store.DesiredService
	for i := range svcs {
		if svcs[i].Name == "app-web" {
			web = &svcs[i]
		}
	}
	if web == nil || web.Health == nil {
		t.Fatalf("web = %+v, want Health carrying the completed dependency", web)
	}
	if got := web.DependsOnCompleted(); len(got) != 1 || got[0] != "migrate" {
		t.Fatalf("DependsOnCompleted = %v, want [migrate]", got)
	}
	if len(web.DependsOnHealthy) != 1 || web.DependsOnHealthy[0] != "db" {
		t.Fatalf("DependsOnHealthy = %v, want [db]", web.DependsOnHealthy)
	}
}
