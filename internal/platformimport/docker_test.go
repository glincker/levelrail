package platformimport

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestDockerSourceDiscover(t *testing.T) {
	data, err := os.ReadFile("testdata/docker/inspect.json")
	if err != nil {
		t.Fatal(err)
	}
	d, err := DockerSource{Data: data}.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Apps) != 2 || len(d.Databases) != 1 || len(d.Unsupported) != 1 {
		t.Fatalf("apps=%d dbs=%d unsupported=%d", len(d.Apps), len(d.Databases), len(d.Unsupported))
	}
	apps := map[string]App{}
	for _, a := range d.Apps {
		apps[a.Name] = a
	}
	web := apps["web"]
	if web.Image != "abcdefghijklmnopqrstuvwx:2222222222222222222222222222222222222222" || web.Replicas != 1 {
		t.Errorf("web picked the wrong container: %+v", web)
	}
	if web.Kind != SourceImage || len(web.Notes) == 0 || !strings.Contains(web.Notes[0].Manual, "docker save") {
		t.Errorf("a host-built image stays an image with a load hint, kind=%s notes=%+v", web.Kind, web.Notes)
	}
	if !web.HostBuilt {
		t.Errorf("a host-built image is not flagged HostBuilt")
	}
	for name, a := range apps {
		if a.HostBuilt && name != "web" {
			t.Errorf("%s flagged host-built with image %q", name, a.Image)
		}
	}
	if web.Port != 3000 || len(web.Domains) != 1 || web.Domains[0] != "app.example.com" {
		t.Errorf("routing: port=%d domains=%v", web.Port, web.Domains)
	}
	if web.Health == nil || web.Health.Path != "/healthz" || web.Health.IntervalSeconds != 5 {
		t.Errorf("health: %+v", web.Health)
	}
	for _, e := range web.Env {
		if e.Key == "PATH" || strings.HasPrefix(e.Key, "COOLIFY_") {
			t.Errorf("platform env leaked: %s", e.Key)
		}
		if e.Key == "DATABASE_URL" && !e.Secret {
			t.Error("DATABASE_URL must be marked secret")
		}
	}
	if len(web.Volumes) != 1 || web.Volumes[0].Name != "web-data" {
		t.Errorf("volumes: %+v", web.Volumes)
	}
	if web.Project != "Main" || web.MemoryBytes != 536870912 {
		t.Errorf("project=%q mem=%d", web.Project, web.MemoryBytes)
	}
	api := apps["api-1"]
	if api.Kind != SourceImage || api.Port != 8080 || len(api.Domains) != 2 {
		t.Errorf("api: %+v", api)
	}
	db := d.Databases[0]
	if db.Engine != "postgres" || db.Version != "17" || db.Name != "main-db" {
		t.Errorf("db: %+v", db)
	}
}

func TestDockerSourceKeepsImageID(t *testing.T) {
	id := "sha256:" + strings.Repeat("ab", 32)
	raw := `[{"Id":"c1","Name":"/web","Image":"` + id + `","Config":{"Image":"abcdefghijklmnopqrstuvwx:latest","ExposedPorts":{"3000/tcp":{}}},"State":{"Running":true}}]`
	d, err := DockerSource{Data: []byte(raw)}.Discover(context.Background())
	if err != nil || len(d.Apps) != 1 {
		t.Fatalf("discover: %v %+v", err, d)
	}
	if a := d.Apps[0]; a.ImageID != id || !a.HostBuilt {
		t.Fatalf("app = %+v", a)
	}
}

func TestDockerSourceRejectsBadInput(t *testing.T) {
	for name, in := range map[string]string{"not json": "nope", "empty": "[]", "object": "{}"} {
		if _, err := (DockerSource{Data: []byte(in)}).Discover(context.Background()); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
