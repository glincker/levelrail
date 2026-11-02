package compose

import (
	"slices"
	"strings"
	"testing"
)

// Shapes of the identity-provider and mail-server templates that failed on start order.
const dependsOnFixture = `
services:
  zitadel:
    image: example/idp:1
    depends_on:
      postgres:
        condition: service_healthy
  postgres:
    image: postgres:16-alpine
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
  migrate:
    image: example/migrate:1
    restart: "no"
    depends_on:
      postgres: {condition: service_healthy}
  web:
    image: example/web:1
    depends_on:
      migrate:
        condition: service_completed_successfully
      zitadel: {condition: service_started}
  worker:
    image: example/worker:1
    depends_on: [web]
`

func TestParseDependsOnConditionsBothForms(t *testing.T) {
	f, err := Parse([]byte(dependsOnFixture))
	if err != nil {
		t.Fatal(err)
	}
	web := f.Services["web"]
	if !slices.Contains(web.DependsOnCompleted, "migrate") || slices.Contains(web.DependsOnHealthy, "zitadel") {
		t.Fatalf("web conditions: healthy=%v completed=%v", web.DependsOnHealthy, web.DependsOnCompleted)
	}
	if got := f.Services["zitadel"].DependsOnHealthy; !slices.Equal(got, []string{"postgres"}) {
		t.Fatalf("zitadel healthy = %v", got)
	}
	if got := []string(f.Services["worker"].DependsOn); !slices.Equal(got, []string{"web"}) {
		t.Fatalf("worker list form = %v", got)
	}
	desired, _, err := ToDesiredServices("idp", f)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range desired {
		if d.Name == "idp-web" && !slices.Equal(d.DependsOnCompleted(), []string{"migrate"}) {
			t.Fatalf("desired web completed = %v", d.DependsOnCompleted())
		}
	}
}

func TestDependsOnCycleNamesTheCycle(t *testing.T) {
	f, err := Parse([]byte("services:\n  a:\n    image: x\n    depends_on: [b]\n  b:\n    image: x\n    depends_on:\n      c: {condition: service_healthy}\n  c:\n    image: x\n    depends_on: [a]\n"))
	if err != nil {
		t.Fatal(err)
	}
	err = f.Validate()
	if err == nil || !strings.Contains(err.Error(), "cycle") || !strings.Contains(err.Error(), "->") {
		t.Fatalf("want a cycle error naming the path, got %v", err)
	}
}
