package compose

import (
	"reflect"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestToDesiredServices_UDPPortsPublishOnHost(t *testing.T) {
	f, err := Parse([]byte(`
services:
  dns:
    image: alpine:3.20
    ports:
      - "53:53/udp"
      - "8080:80"
      - "53:53"
      - target: 3478
        published: 3478
        protocol: udp
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	got, _, err := ToDesiredServices("app", f)
	if err != nil {
		t.Fatalf("ToDesiredServices() error = %v", err)
	}
	if got[0].Port != 80 {
		t.Errorf("Port = %d, want 80 (first tcp port stays the routed one)", got[0].Port)
	}
	want := []store.PublishedPort{
		{ContainerPort: 53, HostPort: 53, Protocol: store.PortProtocolUDP},
		{ContainerPort: 3478, HostPort: 3478, Protocol: store.PortProtocolUDP},
	}
	if !reflect.DeepEqual(got[0].PublishedPorts, want) {
		t.Errorf("PublishedPorts = %+v, want %+v", got[0].PublishedPorts, want)
	}
}

func TestToDesiredServices_UDPPortRejections(t *testing.T) {
	tests := []struct {
		name, ports, want string
	}{
		{"no host port", `["51820/udp"]`, `a host port between 1 and 65535 is required, for example "51820:51820/udp"`},
		{"duplicate", `["53:53/udp", "53:5353/udp"]`, "published port 53/udp is declared twice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse([]byte("services:\n  dns:\n    image: alpine:3.20\n    ports: " + tt.ports + "\n"))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			_, _, err = ToDesiredServices("app", f)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}
