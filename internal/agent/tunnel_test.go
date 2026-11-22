package agent

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/experimental"
)

func TestTunnelCapabilityFollowsFlag(t *testing.T) {
	t.Setenv(experimental.EnvVar, "")
	experimental.Reset()
	defer experimental.Reset()
	if slices.Contains(LocalAgentInfo().GetCapabilities(), CapIngressTunnel) {
		t.Fatal("capability advertised with the flag off")
	}
	t.Setenv(experimental.EnvVar, string(experimental.AgentIngressTunnel))
	experimental.Reset()
	if !slices.Contains(LocalAgentInfo().GetCapabilities(), CapIngressTunnel) {
		t.Fatal("capability missing with the flag on")
	}
}

func TestTunnelProtoRoundTrip(t *testing.T) {
	id := make([]byte, TunnelIDLen)
	id[0] = 7
	msgs := []proto.Message{
		&agentpb.AgentRequest{RequestId: "r", Op: &agentpb.AgentRequest_TunnelOpen{TunnelOpen: &agentpb.TunnelOpenRequest{TunnelId: id, Service: "web", ContainerPort: 8080, DeadlineNs: 5}}},
		&agentpb.AgentResponse{RequestId: "r", Result: &agentpb.AgentResponse_TunnelOpen{TunnelOpen: &agentpb.TunnelOpenResponse{FailureReason: "no match"}}},
		&agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_Hello{Hello: &agentpb.TunnelHello{TunnelId: id}}},
		&agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_Data{Data: make([]byte, TunnelMaxFrameData)}},
		&agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_HalfClose{HalfClose: &agentpb.Empty{}}},
		&agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_Reset_{Reset_: &agentpb.TunnelReset{Reason: "x"}}},
	}
	for _, m := range msgs {
		raw, err := proto.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		got := m.ProtoReflect().New().Interface()
		if err := proto.Unmarshal(raw, got); err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(m, got) {
			t.Fatalf("round trip mismatch for %T", m)
		}
	}
}

func TestTunnelOpenStubRefuses(t *testing.T) {
	resp := Execute(context.Background(), nil, &agentpb.AgentRequest{Op: &agentpb.AgentRequest_TunnelOpen{TunnelOpen: &agentpb.TunnelOpenRequest{}}}, nil)
	if resp.GetError() == "" {
		t.Fatal("tunnel_open must be refused until the data path lands")
	}
}

// Field numbers are wire contract: each must be unique per message and the
// new ones must sit after the previous highest.
func TestTunnelFieldNumbersUnique(t *testing.T) {
	for _, md := range []protoreflect.MessageDescriptor{
		(&agentpb.AgentRequest{}).ProtoReflect().Descriptor(),
		(&agentpb.AgentResponse{}).ProtoReflect().Descriptor(),
		(&agentpb.TunnelOpenRequest{}).ProtoReflect().Descriptor(),
		(&agentpb.TunnelFrame{}).ProtoReflect().Descriptor(),
	} {
		seen := map[protoreflect.FieldNumber]protoreflect.Name{}
		for i := 0; i < md.Fields().Len(); i++ {
			f := md.Fields().Get(i)
			if prev, dup := seen[f.Number()]; dup {
				t.Fatalf("%s: field %d used by %s and %s", md.Name(), f.Number(), prev, f.Name())
			}
			seen[f.Number()] = f.Name()
		}
	}
	req := (&agentpb.AgentRequest{}).ProtoReflect().Descriptor().Fields()
	if n := req.ByName("tunnel_open").Number(); n != 28 || req.ByName("probe").Number() != 27 {
		t.Fatalf("tunnel_open = %d, want 28 after probe 27", n)
	}
	resp := (&agentpb.AgentResponse{}).ProtoReflect().Descriptor().Fields()
	if n := resp.ByName("tunnel_open").Number(); n != 19 || resp.ByName("probe").Number() != 18 {
		t.Fatalf("response tunnel_open = %d, want 19 after probe 18", n)
	}
}

func TestAgentCapabilitiesLoggedOnce(t *testing.T) {
	t.Setenv(experimental.EnvVar, string(experimental.AgentIngressTunnel))
	experimental.Reset()
	defer experimental.Reset()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	var once sync.Once
	logAgentCapabilities(logger, &once)
	logAgentCapabilities(logger, &once)
	out := buf.String()
	if strings.Count(out, "advertised capabilities") != 1 || !strings.Contains(out, CapIngressTunnel) {
		t.Fatalf("log = %q, want one line listing %s", out, CapIngressTunnel)
	}
}

func TestCapSetListSorted(t *testing.T) {
	c := &capSet{}
	c.store([]string{CapIngressTunnel, CapHostProbe})
	if got := c.list(); !slices.Equal(got, []string{CapHostProbe, CapIngressTunnel}) {
		t.Fatalf("list = %v", got)
	}
}
