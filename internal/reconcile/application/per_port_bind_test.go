package application

import (
	"context"
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func mailDesired(bind string) *store.DesiredService {
	d := &store.DesiredService{Name: "mail", Image: "img:v1", Port: 8081, BindAddress: "private", Replicas: 1}
	for _, p := range []int{25, 465, 587, 993} {
		d.PublishedPorts = append(d.PublishedPorts, store.PublishedPort{ContainerPort: p, HostPort: p, Protocol: store.PortProtocolTCP, Bind: bind})
	}
	return d
}

func TestPerPortBind_PublicMailPortsKeepWebPortPrivate(t *testing.T) {
	rt := newFakeRuntime(0)
	c := New("mail", &fakeStore{svc: mailDesired("public")}, rt)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	want := []docker.PortBinding{
		{ContainerPort: 8081, HostIP: "127.0.0.1"},
		{ContainerPort: 25, HostPort: 25, HostIP: "0.0.0.0", Protocol: "tcp"},
		{ContainerPort: 465, HostPort: 465, HostIP: "0.0.0.0", Protocol: "tcp"},
		{ContainerPort: 587, HostPort: 587, HostIP: "0.0.0.0", Protocol: "tcp"},
		{ContainerPort: 993, HostPort: 993, HostIP: "0.0.0.0", Protocol: "tcp"},
	}
	if got := rt.lastCreateSpec.Ports; !reflect.DeepEqual(got, want) {
		t.Fatalf("create ports = %+v, want %+v", got, want)
	}
}

func TestPerPortBind_PrivateDefaultAndInherit(t *testing.T) {
	spec, err := toContainerSpec("mail-1", mailDesired("private"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range spec.Ports {
		if p.HostIP != "127.0.0.1" {
			t.Fatalf("private per-port bind must stay on loopback: %+v", p)
		}
	}
	inherit := udpDesired()
	spec, err = toContainerSpec("dns-1", inherit)
	if err != nil || spec.Ports[1].HostIP != "0.0.0.0" {
		t.Fatalf("a port without its own bind must follow the service bind: %v %+v", err, spec.Ports)
	}
}

func TestPerPortBind_BindChangeIsDrift(t *testing.T) {
	desired := mailDesired("public")
	var private []docker.PortBinding
	for _, p := range desired.PublishedPorts {
		private = append(private, docker.PortBinding{ContainerPort: p.ContainerPort, HostPort: p.HostPort, HostIP: "127.0.0.1", Protocol: "tcp"})
	}
	if !publishedPortsDrifted(&docker.ContainerState{Running: true, Ports: private}, desired) {
		t.Fatal("a port moved to public must read as drift so the container is recreated")
	}
	if publishedPortsDrifted(&docker.ContainerState{Running: true, Ports: private}, mailDesired("private")) {
		t.Fatal("a matching private bind is not drift")
	}
	if publishedPortsDrifted(&docker.ContainerState{Running: true, Ports: private}, mailDesired("")) {
		t.Fatal("a port without its own bind keeps the old host-IP-agnostic check")
	}
}
