package agent

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
)

func TestGRPCTransport_InspectManyByName_OneCall(t *testing.T) {
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))
	type result struct {
		n   int
		web bool
		db  bool
		err error
	}
	done := make(chan result, 1)
	go func() {
		got, err := tr.InspectManyByName(context.Background(), []string{"web", "db", "gone"})
		done <- result{n: len(got), web: got["web"] != nil && got["web"].Running, db: got["db"] != nil, err: err}
	}()
	req := respondToNextCall(t, stream, func(*agentpb.AgentRequest) *agentpb.AgentResponse {
		return &agentpb.AgentResponse{Result: &agentpb.AgentResponse_ListByPrefix{ListByPrefix: &agentpb.ListByPrefixResponse{
			Containers: []*agentpb.ContainerState{{Id: "1", Name: "web", Running: true}, {Id: "2", Name: "db"}, {Id: "3", Name: "other"}},
		}}}
	})
	if req.GetListByPrefix() == nil || req.GetListByPrefix().GetPrefix() != "" {
		t.Fatalf("want one unprefixed list call, got %v", req)
	}
	r := <-done
	if r.err != nil || r.n != 3 || !r.web || !r.db {
		t.Fatalf("result = %+v", r)
	}
}
