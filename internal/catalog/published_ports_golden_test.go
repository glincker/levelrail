package catalog

import (
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// legacySplit is the port split every template got before published tcp
// ports existed: first tcp port routed, udp ports published, other tcp dropped.
func legacySplit(ports []compose.Port) (int, []spec.PublishedPort) {
	main := 0
	var out []spec.PublishedPort
	for _, p := range ports {
		if p.Protocol != spec.PortProtocolUDP {
			if main == 0 {
				main = p.ContainerPort
			}
			continue
		}
		out = append(out, spec.PublishedPort{ContainerPort: p.ContainerPort, HostPort: p.HostPort, Protocol: spec.PortProtocolUDP})
	}
	return main, out
}

func TestTemplatesWithoutPublishedTCPResolveAsBefore(t *testing.T) {
	multi := 0
	for _, tpl := range Templates {
		f, err := compose.Parse([]byte(tpl.Compose))
		if err != nil {
			t.Fatalf("%s: %v", tpl.ID, err)
		}
		svcs, _, err := compose.ToDesiredServices("g", f)
		if err != nil {
			continue
		}
		for _, d := range svcs {
			key := d.Name[len("g-"):]
			src := f.Services[key]
			if len(src.PublishedTCP) > 0 {
				continue
			}
			if len(src.Ports) > 1 {
				multi++
			}
			main, published := legacySplit(src.Ports)
			if d.Port != main || !reflect.DeepEqual(d.PublishedPorts, published) {
				t.Errorf("%s/%s changed: port %d %+v, want %d %+v", tpl.ID, key, d.Port, d.PublishedPorts, main, published)
			}
		}
	}
	if multi < 3 {
		t.Fatalf("expected several multi-port templates in the golden set, got %d", multi)
	}
}

func TestMailTemplatePublishesFourPortsPrivately(t *testing.T) {
	f, err := compose.Parse([]byte(mailMoxTemplate().Compose))
	if err != nil {
		t.Fatal(err)
	}
	svcs, _, err := compose.ToDesiredServices("mail", f)
	if err != nil || len(svcs) != 1 {
		t.Fatalf("%v %d", err, len(svcs))
	}
	d := svcs[0]
	if d.Port != 8081 {
		t.Fatalf("routed web port = %d, want 8081", d.Port)
	}
	var got []int
	for _, p := range d.PublishedPorts {
		if p.Bind != spec.PortBindPrivate || p.EffectiveProtocol() != spec.PortProtocolTCP || p.ContainerPort == 8081 {
			t.Fatalf("mail port must be tcp with a private default: %+v", p)
		}
		got = append(got, p.HostPort)
	}
	if !reflect.DeepEqual(got, []int{25, 465, 587, 993}) {
		t.Fatalf("published = %v", got)
	}
}
