package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestPublishedPortsOutput(t *testing.T) {
	dns := []publishedPort{{ContainerPort: 53, HostPort: 53, Protocol: "udp"}, {ContainerPort: 53, HostPort: 53}}
	tests := []struct {
		name string
		run  func(b *bytes.Buffer)
		want string
	}{
		{"show", func(b *bytes.Buffer) { printAppHuman(b, appResource{Name: "dns", PublishedPorts: dns}) }, "published: 53/udp, 53/tcp"},
		{"list", func(b *bytes.Buffer) { printAppsTable(b, []appResource{{Name: "dns", Port: 80, PublishedPorts: dns}}) }, "80 +53/udp, 53/tcp"},
		{"compose", func(b *bytes.Buffer) {
			printComposeDeployResultHuman(b, composeDeployResult{AppID: "a", Services: []appResource{{Name: "dns", PublishedPorts: dns}}})
		}, "published: 53/udp, 53/tcp"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			tc.run(&b)
			if !strings.Contains(b.String(), tc.want) {
				t.Fatalf("missing %q in %q", tc.want, b.String())
			}
		})
	}
	raw, _ := json.Marshal(appResource{Name: "dns", PublishedPorts: dns[:1]})
	if !strings.Contains(string(raw), `"published_ports":[{"container_port":53,"host_port":53,"protocol":"udp"}]`) {
		t.Fatalf("json: %s", raw)
	}
}

func TestPlanFromFileSendsPublishedPorts(t *testing.T) {
	fileSpec, err := spec.Parse([]byte(`version: 1
services:
  dns:
    port: 80
    build:
      type: image
      image: coredns/coredns:1.11.1
    published_ports:
      - container_port: 53
        host_port: 5353
        protocol: udp
      - container_port: 53
        host_port: 5353
`))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planFromFile(createFlags{file: "app.yaml"}, fileSpec, detectedGit{})
	if err != nil {
		t.Fatal(err)
	}
	want := []publishedPort{{ContainerPort: 53, HostPort: 5353, Protocol: "udp"}, {ContainerPort: 53, HostPort: 5353, Protocol: "tcp"}}
	if !reflect.DeepEqual(plan.CreateBody.PublishedPorts, want) {
		t.Fatalf("got %v want %v", plan.CreateBody.PublishedPorts, want)
	}
}
