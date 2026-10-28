package store

import (
	"context"
	"reflect"
	"testing"
)

func TestSaveDesiredService_PublishedPortsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "plain", Image: "nginx:1.27", Port: 80}); err != nil {
		t.Fatalf("SaveDesiredService(plain) error = %v", err)
	}
	plain, err := db.GetDesiredService(ctx, "plain")
	if err != nil {
		t.Fatalf("GetDesiredService(plain) error = %v", err)
	}
	if plain.PublishedPorts != nil {
		t.Errorf("plain.PublishedPorts = %+v, want nil", plain.PublishedPorts)
	}

	svc := DesiredService{Name: "dns", Image: "alpine:3.20", Port: 53, PublishedPorts: []PublishedPort{
		{ContainerPort: 53, HostPort: 53, Protocol: PortProtocolUDP},
		{ContainerPort: 9000, HostPort: 9000},
	}}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatalf("SaveDesiredService(dns) error = %v", err)
	}
	got, err := db.GetDesiredService(ctx, "dns")
	if err != nil {
		t.Fatalf("GetDesiredService(dns) error = %v", err)
	}
	want := []PublishedPort{
		{ContainerPort: 53, HostPort: 53, Protocol: PortProtocolUDP},
		{ContainerPort: 9000, HostPort: 9000, Protocol: PortProtocolTCP},
	}
	if !reflect.DeepEqual(got.PublishedPorts, want) {
		t.Errorf("PublishedPorts = %+v, want %+v", got.PublishedPorts, want)
	}
}
