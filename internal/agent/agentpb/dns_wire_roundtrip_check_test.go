package agentpb

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// The dns field was added to the generated descriptor by hand
// (protoc-gen-go was unavailable in the environment that made this
// change). internal/agent/convert_test.go's own round-trip test only
// exercises containerSpecToPB/FromPB at the Go-struct level, which
// would still pass even if the rawDesc byte string were subtly
// malformed. This test proves the field survives a real wire-format
// marshal/unmarshal and a real protojson marshal/unmarshal, the two
// paths that would actually surface a broken descriptor edit.
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
