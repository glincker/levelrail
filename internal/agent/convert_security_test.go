package agent

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestContainerSpecSecurityFieldsRoundTrip(t *testing.T) {
	in := docker.ContainerSpec{
		Name: "b", Image: "i", UsernsMode: "auto", SecurityOpt: []string{"seccomp=unconfined"},
		Devices: []string{"/dev/fuse"}, User: "1000:1000",
		BindMounts: []docker.BindMount{{HostPath: "/run/x", ContainerPath: "/run/y", ReadOnly: true}},
	}
	raw, err := proto.Marshal(containerSpecToPB(in))
	if err != nil {
		t.Fatal(err)
	}
	var pb agentpb.ContainerSpec
	if err := proto.Unmarshal(raw, &pb); err != nil {
		t.Fatal(err)
	}
	if out := containerSpecFromPB(&pb); !reflect.DeepEqual(out, in) {
		t.Fatalf("round trip: %+v", out)
	}
}

func TestOldAgentSpecWithoutNewFieldsDecodes(t *testing.T) {
	raw, err := proto.Marshal(&agentpb.ContainerSpec{Name: "a", Image: "i"})
	if err != nil {
		t.Fatal(err)
	}
	var pb agentpb.ContainerSpec
	if err := proto.Unmarshal(raw, &pb); err != nil {
		t.Fatal(err)
	}
	if got := containerSpecFromPB(&pb); got.NeedsSecurityCapability() {
		t.Fatalf("plain spec needs capability: %+v", got)
	}
}

func TestCreateRefusedForAgentWithoutCapability(t *testing.T) {
	tr := newGRPCTransport(nil)
	_, err := tr.Create(context.Background(), docker.ContainerSpec{Name: "b", Devices: []string{"/dev/fuse"}})
	if !errors.Is(err, ErrAgentLacksCapability) {
		t.Fatalf("err %v", err)
	}
	tr.caps.store([]string{CapContainerSpecSecurity})
	if !tr.caps.has(CapContainerSpecSecurity) {
		t.Fatal("capability not recorded")
	}
}

func TestLocalAgentInfoAdvertisesSecurityCapability(t *testing.T) {
	caps := LocalAgentInfo().GetCapabilities()
	want := []string{CapContainerSpecSecurity, CapExitTimes}
	if !slices.Equal(caps, want) {
		t.Fatalf("caps %v, want exactly %v", caps, want)
	}
}
