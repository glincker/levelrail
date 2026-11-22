package platformimport

import (
	"strings"
	"testing"
)

const flyFixture = `app = "shop-api"
primary_region = "ams"
kill_signal = "SIGINT"

[build]
  image = "ghcr.io/acme/shop:1.4"

[env]
  PORT = "8080"
  API_TOKEN = "abc123"

[deploy]
  release_command = "npm run migrate"

[http_service]
  internal_port = 8080
  force_https = true
  min_machines_running = 2
  [[http_service.checks]]
    path = "/healthz"

[[vm]]
  memory = "1gb"
  cpus = 2

[[mounts]]
  source = "data"
  destination = "/data"
`

func TestFlyTOMLMapsAppSettings(t *testing.T) {
	in, err := FlyTOMLToEmit([]byte(flyFixture), "fallback", "fly.toml")
	if err != nil {
		t.Fatal(err)
	}
	a := in.Apps[0]
	if a.Name != "shop-api" || a.Image != "ghcr.io/acme/shop:1.4" || a.Port != 8080 || a.Replicas != 2 {
		t.Fatalf("app %+v", a)
	}
	if a.HealthPath != "/healthz" || a.PreDeploy != "npm run migrate" || a.Memory != "1024Mi" || a.NanoCPUs != 2e9 {
		t.Fatalf("app %+v", a)
	}
	notes := strings.Join(in.Notes, "\n")
	for _, want := range []string{"primary_region ams", "kill_signal has no equivalent", "mounts has no direct equivalent", "force_https"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes missing %q:\n%s", want, notes)
		}
	}
	files, err := EmitStack(in, EmitOptions{Stack: "shop", Project: "shop"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no files emitted")
	}
	if _, err := FlyTOMLToEmit([]byte("[[build]\n"), "x", "fly.toml"); err == nil {
		t.Error("a malformed header should fail")
	}
}
