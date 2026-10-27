package application

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func udpDesired() *store.DesiredService {
	return &store.DesiredService{Name: "dns", Image: "img:v1", Port: 53, BindAddress: "public", Replicas: 1,
		PublishedPorts: []store.PublishedPort{{ContainerPort: 53, HostPort: 53, Protocol: store.PortProtocolUDP}}}
}

func TestPublishedPorts_CreateBindsUDPOnHost(t *testing.T) {
	rt := newFakeRuntime(0)
	c := New("dns", &fakeStore{svc: udpDesired()}, rt)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	want := []docker.PortBinding{
		{ContainerPort: 53, HostIP: "0.0.0.0"},
		{ContainerPort: 53, HostPort: 53, HostIP: "0.0.0.0", Protocol: "udp"},
	}
	if got := rt.lastCreateSpec.Ports; !reflect.DeepEqual(got, want) {
		t.Fatalf("create ports = %+v, want %+v", got, want)
	}
}

func TestPublishedPorts_RemoteNodeRefused(t *testing.T) {
	desired := udpDesired()
	desired.NodeID = "node-b"
	rt := newFakeRuntime(0)
	c := New("dns", &fakeStore{svc: desired}, rt)
	_, err := c.Reconcile(context.Background())
	if err == nil || !strings.Contains(err.Error(), "host port 53/udp can only be published on the control plane's own node") {
		t.Fatalf("Reconcile() error = %v, want the remote node refusal", err)
	}
	if rt.createCalls != 0 {
		t.Fatalf("createCalls = %d, want 0", rt.createCalls)
	}
}

func TestPublishedPortsDrifted(t *testing.T) {
	desired := udpDesired()
	tcp53 := docker.PortBinding{ContainerPort: 53, HostPort: 32768, HostIP: "0.0.0.0", Protocol: "tcp"}
	udp53 := docker.PortBinding{ContainerPort: 53, HostPort: 53, HostIP: "0.0.0.0", Protocol: "udp"}
	tests := []struct {
		name  string
		ports []docker.PortBinding
		want  bool
	}{
		{"matches", []docker.PortBinding{tcp53, udp53}, false},
		{"protocol changed to tcp", []docker.PortBinding{tcp53, {ContainerPort: 53, HostPort: 53, Protocol: "tcp"}}, true},
		{"udp missing", []docker.PortBinding{tcp53}, true},
		{"unknown", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &docker.ContainerState{Running: true, Ports: tt.ports}
			if got := publishedPortsDrifted(state, desired); got != tt.want {
				t.Fatalf("publishedPortsDrifted() = %v, want %v", got, tt.want)
			}
		})
	}
	stale := &docker.ContainerState{Running: true, Ports: []docker.PortBinding{tcp53, udp53}}
	if !publishedPortsDrifted(stale, &store.DesiredService{Port: 53}) {
		t.Fatal("a removed udp port must read as drift")
	}
}
