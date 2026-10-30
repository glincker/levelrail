package deploy

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestPipeline_Deploy_PreviewDropsHostPorts(t *testing.T) {
	st := &fakeServiceStore{}
	p := New(&fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}, st)
	svc := dockerfileService()
	svc.HostPort = 8080
	svc.PublishedPorts = []spec.PublishedPort{{ContainerPort: 53, HostPort: 53, Protocol: spec.PortProtocolUDP}}
	if _, err := p.Deploy(context.Background(), Request{ServiceName: "web-pr-7", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web", Preview: true}, nil); err != nil {
		t.Fatal(err)
	}
	if st.saved.HostPort != nil || len(st.saved.PublishedPorts) != 0 {
		t.Fatalf("preview kept the main app's host ports: host_port=%v published=%v", st.saved.HostPort, st.saved.PublishedPorts)
	}
}
