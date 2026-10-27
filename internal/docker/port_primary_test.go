package docker

import "testing"

func TestPrimaryPortSkipsUDP(t *testing.T) {
	got, ok := PrimaryPort([]PortBinding{{ContainerPort: 53, HostPort: 53, Protocol: "udp"}, {ContainerPort: 80, HostPort: 32768}})
	if !ok || got.HostPort != 32768 {
		t.Fatalf("PrimaryPort = %+v %v, want the tcp binding", got, ok)
	}
	if _, ok := PrimaryPort([]PortBinding{{HostPort: 53, Protocol: "udp"}}); ok {
		t.Fatal("PrimaryPort picked a udp-only binding")
	}
}
