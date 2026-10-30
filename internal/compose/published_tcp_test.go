package compose

import (
	"errors"
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestSplitPortsPublishedTCP(t *testing.T) {
	ports := []Port{{HostPort: 8025, ContainerPort: 8081}, {HostPort: 25, ContainerPort: 25}, {HostPort: 2525, ContainerPort: 587}, {HostPort: 9000, ContainerPort: 9000}}
	main, published, err := splitPorts(ports, []int{25, 587})
	if err != nil {
		t.Fatal(err)
	}
	want := []spec.PublishedPort{
		{ContainerPort: 25, HostPort: 25, Protocol: spec.PortProtocolTCP, Bind: spec.PortBindPrivate},
		{ContainerPort: 587, HostPort: 2525, Protocol: spec.PortProtocolTCP, Bind: spec.PortBindPrivate},
	}
	if main != 8081 || !reflect.DeepEqual(published, want) {
		t.Fatalf("got main %d published %+v", main, published)
	}
	main, published, err = splitPorts(ports, nil)
	if err != nil || main != 8081 || len(published) != 0 {
		t.Fatalf("without the extension extra tcp ports stay dropped: %d %+v %v", main, published, err)
	}
}

func TestSplitPortsPublishedTCPRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		ports []Port
		list  []int
	}{
		"not declared":   {[]Port{{HostPort: 80, ContainerPort: 80}}, []int{25}},
		"no host port":   {[]Port{{ContainerPort: 80}, {ContainerPort: 25}}, []int{25}},
		"udp is not tcp": {[]Port{{HostPort: 53, ContainerPort: 53, Protocol: spec.PortProtocolUDP}}, []int{53}},
	} {
		if _, _, err := splitPorts(tc.ports, tc.list); !errors.Is(err, spec.ErrInvalidPublishedPort) {
			t.Fatalf("%s: want ErrInvalidPublishedPort, got %v", name, err)
		}
	}
}

func TestParsePublishedTCPExtension(t *testing.T) {
	f, err := Parse([]byte("services:\n  web:\n    image: x:1\n    ports: [\"8080:80\", \"25:25\"]\n    x-levelrail-published-ports: [25]\n"))
	if err != nil {
		t.Fatal(err)
	}
	svcs, _, err := ToDesiredServices("a", f)
	if err != nil {
		t.Fatal(err)
	}
	if svcs[0].Port != 80 || len(svcs[0].PublishedPorts) != 1 || svcs[0].PublishedPorts[0].Bind != spec.PortBindPrivate {
		t.Fatalf("unexpected %+v", svcs[0])
	}
}
