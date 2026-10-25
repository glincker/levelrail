package agent

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

type logRuntime struct {
	*execRuntime
	lines []docker.LogLine
	gotID string
	since time.Time
}

func (l *logRuntime) Logs(_ context.Context, id string, follow bool, since time.Time) (<-chan docker.LogLine, <-chan error) {
	l.gotID, l.since = id, since
	out := make(chan docker.LogLine, len(l.lines))
	errCh := make(chan error)
	for _, ln := range l.lines {
		out <- ln
	}
	close(out)
	close(errCh)
	_ = follow
	return out, errCh
}

func TestExecute_Logs_OrdersByTimeAndCarriesCursor(t *testing.T) {
	t0 := time.Unix(1000, 0)
	rt := &logRuntime{execRuntime: newExecRuntime(), lines: []docker.LogLine{
		{Stream: "stderr", Timestamp: t0.Add(2 * time.Second), Message: "b"},
		{Stream: "stdout", Timestamp: t0.Add(time.Second), Message: "a"},
	}}
	resp := Execute(context.Background(), rt, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_Logs{Logs: &agentpb.LogsRequest{ContainerId: "c1", SinceUnixNano: t0.UnixNano()}},
	}, nil)
	if resp.Error != "" {
		t.Fatalf("error = %q", resp.Error)
	}
	got := resp.GetLogs().GetLines()
	if len(got) != 2 || got[0].Message != "a" || got[1].Message != "b" {
		t.Fatalf("lines = %v, want a then b", got)
	}
	if rt.gotID != "c1" || !rt.since.Equal(t0) {
		t.Errorf("runtime got id=%q since=%v", rt.gotID, rt.since)
	}
}

func TestExecute_Logs_CapsBatch(t *testing.T) {
	t.Setenv(envAgentLogBatchMax, "2")
	var lines []docker.LogLine
	for i := 0; i < 5; i++ {
		lines = append(lines, docker.LogLine{Stream: "stdout", Timestamp: time.Unix(int64(i+1), 0), Message: "x"})
	}
	rt := &logRuntime{execRuntime: newExecRuntime(), lines: lines}
	resp := Execute(context.Background(), rt, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_Logs{Logs: &agentpb.LogsRequest{ContainerId: "c1"}},
	}, nil)
	if n := len(resp.GetLogs().GetLines()); n != 2 {
		t.Fatalf("lines = %d, want capped at 2", n)
	}
}

func TestExecute_Logs_UnsupportedRuntime(t *testing.T) {
	resp := Execute(context.Background(), newExecRuntime(), &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_Logs{Logs: &agentpb.LogsRequest{ContainerId: "c1"}},
	}, nil)
	if resp.Error != ErrLogsUnsupported.Error() {
		t.Fatalf("error = %q, want unsupported", resp.Error)
	}
}

func TestContainerStateLabelsRoundTrip(t *testing.T) {
	in := &docker.ContainerState{ID: "1", Labels: map[string]string{"levelrail.tls-cert": "ecdsa"}}
	out := containerStateFromPB(containerStateToPB(in))
	if out.LabelsUnknown || out.Labels["levelrail.tls-cert"] != "ecdsa" {
		t.Fatalf("round trip = %+v", out)
	}
	if !containerStateFromPB(&agentpb.ContainerState{Id: "old"}).LabelsUnknown {
		t.Error("a state from an agent that predates labels must read as unknown")
	}
}
