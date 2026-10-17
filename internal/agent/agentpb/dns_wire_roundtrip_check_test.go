package agentpb

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Exercises real wire and protojson marshaling, which a broken hand-edited descriptor would fail.
func TestDNSFieldSurvivesRealWireRoundTrip(t *testing.T) {
	spec := &ContainerSpec{
		Name:  "test",
		Image: "nginx",
		Dns:   []string{"10.0.0.1", "10.0.0.2"},
	}

	wireBytes, err := proto.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded ContainerSpec
	if err := proto.Unmarshal(wireBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(decoded.Dns) != 2 || decoded.Dns[0] != "10.0.0.1" || decoded.Dns[1] != "10.0.0.2" {
		t.Fatalf("DNS field did not survive wire round trip: got %v", decoded.Dns)
	}

	jsonBytes, err := protojson.Marshal(spec)
	if err != nil {
		t.Fatalf("protojson marshal failed: %v", err)
	}
	t.Logf("protojson: %s", jsonBytes)
	var decoded2 ContainerSpec
	if err := protojson.Unmarshal(jsonBytes, &decoded2); err != nil {
		t.Fatalf("protojson unmarshal failed: %v", err)
	}
	if len(decoded2.Dns) != 2 {
		t.Fatalf("DNS field did not survive protojson round trip: got %v", decoded2.Dns)
	}
}
