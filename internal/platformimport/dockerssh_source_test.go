package platformimport

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

type fakeInspect struct {
	data []byte
	err  error
}

func (f fakeInspect) InspectAll(context.Context) ([]byte, error) { return f.data, f.err }

func TestDockerSSHSourceDiscover(t *testing.T) {
	data, err := os.ReadFile("testdata/docker/inspect.json")
	if err != nil {
		t.Fatal(err)
	}
	d, err := DockerSSHSource{Reader: fakeInspect{data: data}}.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d.Platform != DockerSSH || len(d.Apps) != 2 {
		t.Fatalf("platform=%s apps=%d", d.Platform, len(d.Apps))
	}
	for _, a := range d.Apps {
		last := a.Notes[len(a.Notes)-1]
		if !strings.Contains(last.Reason, "stored config was not read") {
			t.Errorf("%s has no stored-config note: %+v", a.Name, a.Notes)
		}
	}
	running := 0
	for _, a := range d.Apps {
		if a.SourceContainer != "" {
			running++
		}
	}
	if running == 0 {
		t.Error("no app carries its source container id")
	}
}

func TestDockerSSHSourceErrors(t *testing.T) {
	if _, err := (DockerSSHSource{}).Discover(context.Background()); err == nil {
		t.Error("nil reader accepted")
	}
	boom := errors.New("boom")
	if _, err := (DockerSSHSource{Reader: fakeInspect{err: boom}}).Discover(context.Background()); !errors.Is(err, boom) {
		t.Errorf("read error not wrapped: %v", err)
	}
}

func TestDockerSkipsOtherPlatformsInfrastructure(t *testing.T) {
	raw := `[{"Id":"1","Name":"/captain-nginx","Config":{"Image":"nginx:1"},"State":{"Running":true}},
	{"Id":"2","Name":"/dokploy-traefik","Config":{"Image":"traefik:3"},"State":{"Running":true}},
	{"Id":"3","Name":"/shop","Config":{"Image":"shop:1","ExposedPorts":{"80/tcp":{}}},"State":{"Running":true}}]`
	d, err := DockerSource{Data: []byte(raw)}.Discover(context.Background())
	if err != nil || len(d.Apps) != 1 || d.Apps[0].Name != "shop" {
		t.Fatalf("apps = %+v, %v", d, err)
	}
}

func TestParsePlatformAcceptsDockerSSH(t *testing.T) {
	if p, err := ParsePlatform("docker-ssh"); err != nil || p != DockerSSH {
		t.Fatalf("ParsePlatform = %v, %v", p, err)
	}
}
