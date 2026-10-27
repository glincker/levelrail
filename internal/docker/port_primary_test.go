package docker

import "testing"

func TestPrimaryPortSkipsUDP(t *testing.T) {
	got, ok := PrimaryPort([]PortBinding{{ContainerPort: 53, HostPort: 53, Protocol: "udp"}, {ContainerPort: 80, HostPort: 32768}}, 0)
	if !ok || got.HostPort != 32768 {
		t.Fatalf("PrimaryPort = %+v %v, want the tcp binding", got, ok)
	}
	if _, ok := PrimaryPort([]PortBinding{{ContainerPort: 53, HostPort: 53, Protocol: "udp"}}, 53); ok {
		t.Fatal("PrimaryPort picked a udp-only binding")
	}
}

func TestPrimaryPortPicksDesiredAmongShuffled(t *testing.T) {
	web := PortBinding{ContainerPort: 3000, HostPort: 32768, Protocol: "tcp"}
	ssh := PortBinding{ContainerPort: 22, HostPort: 2222, Protocol: "tcp"}
	dns := PortBinding{ContainerPort: 53, HostPort: 53, Protocol: "udp"}
	for _, order := range [][]PortBinding{{ssh, web, dns}, {dns, ssh, web}, {web, ssh, dns}, {ssh, dns, web}} {
		got, ok := PrimaryPort(order, 3000)
		if !ok || got != web {
			t.Fatalf("PrimaryPort(%v, 3000) = %+v %v, want the 3000 binding", order, got, ok)
		}
	}
	if _, ok := PrimaryPort([]PortBinding{ssh, dns}, 3000); ok {
		t.Fatal("PrimaryPort fell back to another port while the desired one is unpublished")
	}
}
