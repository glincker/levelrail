package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// inspectFakeRuntime ignores the query's label filter on purpose, so the
// agent's own scope recheck is what the tests exercise.
type inspectFakeRuntime struct {
	docker.Runtime
	details []docker.ContainerDetail
	got     docker.DetailQuery
}

func (f *inspectFakeRuntime) InspectContainerDetails(_ context.Context, q docker.DetailQuery) ([]docker.ContainerDetail, error) {
	f.got = q
	return f.details, nil
}

func inspectReq(instance, prefix string, limit uint32) *agentpb.AgentRequest {
	return &agentpb.AgentRequest{RequestId: "r1", Op: &agentpb.AgentRequest_InspectContainers{
		InspectContainers: &agentpb.InspectContainersRequest{InstanceId: instance, NamePrefix: prefix, Limit: limit},
	}}
}

func TestInspectContainers_OnlyInstanceLabelled(t *testing.T) {
	rt := &inspectFakeRuntime{details: []docker.ContainerDetail{
		{ID: "mine", Name: "web-abc", InstanceID: "cp-1", State: "running", Health: &docker.ContainerHealthDetail{Status: "healthy"},
			Networks: []docker.ContainerNetworkDetail{{Name: "lr", IP: "10.0.0.2", Aliases: []string{"web"}}}},
		{ID: "tenant-b", Name: "web-xyz", InstanceID: "cp-2"},
		{ID: "unlabelled", Name: "web-old"},
		{ID: "other-app", Name: "db-1", InstanceID: "cp-1"},
	}}
	resp := Execute(context.Background(), rt, inspectReq("cp-1", "web-", 0), nil)
	if resp.GetError() != "" {
		t.Fatalf("error = %s", resp.GetError())
	}
	got := resp.GetInspectContainers().GetContainers()
	if len(got) != 1 || got[0].GetId() != "mine" || got[0].GetHealth().GetStatus() != "healthy" || got[0].GetNetworks()[0].GetAliases()[0] != "web" {
		t.Fatalf("containers = %v", got)
	}
	if rt.got.LabelKey != spec.InstanceLabelKey || rt.got.LabelValue != "cp-1" {
		t.Errorf("query = %+v", rt.got)
	}
	raw, _ := proto.Marshal(resp)
	for _, leak := range []string{"tenant-b", "unlabelled", "cp-2"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("response carries %q", leak)
		}
	}
}

func TestInspectContainers_RefusesUnscoped(t *testing.T) {
	rt := &inspectFakeRuntime{details: []docker.ContainerDetail{{ID: "x", Name: "web-1", InstanceID: ""}}}
	for _, req := range []*agentpb.AgentRequest{inspectReq("", "web-", 0), inspectReq("cp-1", "", 0)} {
		if resp := Execute(context.Background(), rt, req, nil); resp.GetError() == "" || resp.GetInspectContainers() != nil {
			t.Errorf("unscoped request answered: %v", resp)
		}
	}
}

func TestInspectContainers_Bounds(t *testing.T) {
	t.Setenv(envInspectMaxContainers, "3")
	var many []docker.ContainerDetail
	for i := 0; i < 10; i++ {
		many = append(many, docker.ContainerDetail{ID: strings.Repeat(string(rune('a'+i)), 20), Name: "web-r", InstanceID: "cp-1"})
	}
	rt := &inspectFakeRuntime{details: many}
	resp := Execute(context.Background(), rt, inspectReq("cp-1", "web-", 100), nil)
	if n := len(resp.GetInspectContainers().GetContainers()); n != 3 || !resp.GetInspectContainers().GetTruncated() || rt.got.Limit != 3 {
		t.Fatalf("count bound: n = %d truncated = %v limit = %d", n, resp.GetInspectContainers().GetTruncated(), rt.got.Limit)
	}
	t.Setenv(envInspectMaxBytes, "40")
	resp = Execute(context.Background(), rt, inspectReq("cp-1", "web-", 0), nil)
	if n := len(resp.GetInspectContainers().GetContainers()); n >= 3 || !resp.GetInspectContainers().GetTruncated() {
		t.Fatalf("byte bound: n = %d", n)
	}
}

func TestInspectContainers_OldAgentNeverReceivesCall(t *testing.T) {
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))
	_, err := tr.InspectContainerDetails(context.Background(), docker.DetailQuery{NamePrefix: "web-", LabelValue: "cp-1"})
	if !errors.Is(err, docker.ErrDetailUnsupported) {
		t.Fatalf("err = %v", err)
	}
	if len(stream.sent) != 0 {
		t.Fatal("old agent was sent an inspect request")
	}
}

func TestInspectContainers_TransportRoundTrip(t *testing.T) {
	stream := newFakeSessionStream()
	caps := &capSet{}
	caps.store([]string{CapContainerInspect})
	tr := newGRPCTransportWithCaps(newMux(stream), caps)
	started := time.Now().UTC().Truncate(time.Second)
	rt := &inspectFakeRuntime{details: []docker.ContainerDetail{{ID: "c1", Name: "web-1", InstanceID: "cp-1", RestartCount: 4, StartedAt: started,
		Mounts: []docker.ContainerMountDetail{{Type: "volume", Source: "web-data", Target: "/data"}}, NanoCPUs: 5e8, MemoryBytes: 1 << 29}}}

	done := make(chan []docker.ContainerDetail, 1)
	go func() {
		out, _ := tr.InspectContainerDetails(context.Background(), docker.DetailQuery{NamePrefix: "web-", LabelValue: "cp-1", Limit: 2})
		done <- out
	}()
	var req *agentpb.AgentRequest
	select {
	case msg := <-stream.sent:
		req = msg.GetRequest()
	case <-time.After(2 * time.Second):
		t.Fatal("no request sent")
	}
	if req.GetInspectContainers().GetLimit() != 2 || req.GetInspectContainers().GetInstanceId() != "cp-1" {
		t.Fatalf("request = %v", req)
	}
	stream.recv <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{Response: Execute(context.Background(), rt, req, nil)}}
	select {
	case out := <-done:
		if len(out) != 1 || out[0].RestartCount != 4 || !out[0].StartedAt.Equal(started) || out[0].Mounts[0].Source != "web-data" || out[0].InstanceID != "cp-1" {
			t.Fatalf("details = %+v", out)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call did not return")
	}
}

func TestInspectContainers_IDsNarrowBeforeLimit(t *testing.T) {
	rt := &inspectFakeRuntime{details: []docker.ContainerDetail{
		{ID: "sib", Name: "web-0-aaa", InstanceID: "cp-1"},
		{ID: "own", Name: "web-bbb", InstanceID: "cp-1"},
	}}
	req := inspectReq("cp-1", "web-", 1)
	req.GetInspectContainers().ContainerIds = []string{"own"}
	got := Execute(context.Background(), rt, req, nil).GetInspectContainers().GetContainers()
	if len(got) != 1 || got[0].GetId() != "own" || len(rt.got.IDs) != 1 {
		t.Fatalf("containers = %v query = %+v", got, rt.got)
	}
}

func TestInspectContainers_ControlPlaneCapsAgentData(t *testing.T) {
	t.Setenv(envInspectMaxBytes, "999999999")
	if b := min(agentEnvInt(envInspectMaxBytes, defaultInspectMaxBytes), ceilingInspectMaxBytes); b != ceilingInspectMaxBytes {
		t.Fatalf("budget = %d", b)
	}
	pb := &agentpb.ContainerDetail{Id: "c", Health: &agentpb.ContainerHealthDetail{LastOutput: strings.Repeat("y", 5000)}}
	for i := 0; i < 200; i++ {
		pb.Networks = append(pb.Networks, &agentpb.ContainerNetworkDetail{Name: "n", Aliases: make([]string, 100)})
		pb.Mounts = append(pb.Mounts, &agentpb.ContainerMountDetail{Type: "volume"})
	}
	d := containerDetailFromPB(pb, "cp-1")
	if len(d.Health.LastOutput) != docker.HealthOutputMaxBytes || len(d.Networks) != maxDetailNetworks ||
		len(d.Networks[0].Aliases) != maxDetailAliases || len(d.Mounts) != maxDetailMounts {
		t.Fatalf("caps not applied: out=%d nets=%d aliases=%d mounts=%d", len(d.Health.LastOutput), len(d.Networks), len(d.Networks[0].Aliases), len(d.Mounts))
	}
}
