package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPersonalBuilderEnrolsBuildOnlyAndStaysOffProduction(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	st := newFakeEnrollStore()
	seedJoinToken(st, "builder-token", time.Hour)
	st.tokens[hashJoinToken("builder-token")].BuildTrust = store.BuildTrustPersonal
	srv := NewServer(ca, st, NewRegistry(), nil)

	resp, err := srv.Enroll(context.Background(), &agentpb.EnrollRequest{CsrDer: mustCSR(t), JoinToken: "builder-token", NodeName: "laptop"})
	if err != nil {
		t.Fatalf("Enroll() error = %v", err)
	}
	n := st.nodes[resp.GetNodeId()]
	if n.AcceptsAppWorkloads || !n.AcceptsBuildWorkloads {
		t.Fatalf("personal builder flags app=%v build=%v, want build-only", n.AcceptsAppWorkloads, n.AcceptsBuildWorkloads)
	}
	if n.BuildTrust != store.BuildTrustPersonal {
		t.Fatalf("tier = %q, want personal", n.BuildTrust)
	}

	info := []build.NodeInfo{{ID: n.ID, Name: n.Name, AcceptsBuildWorkloads: true, Online: true, Trust: n.BuildTrust}}
	if d := build.ChooseBuildNode(info, build.Query{App: "web", EnvKind: "production"}, build.Policy{Now: time.Now()}); d.Node != "" {
		t.Fatalf("personal builder chosen for production: %+v", d)
	}
}

func TestPersonalBuilderEnrolmentFailsWhenNodeWriteFails(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	st := newFakeEnrollStore()
	st.saveNodeErr = errors.New("write failed")
	seedJoinToken(st, "builder-token", time.Hour)
	st.tokens[hashJoinToken("builder-token")].BuildTrust = store.BuildTrustPersonal
	srv := NewServer(ca, st, NewRegistry(), nil)

	if _, err := srv.Enroll(context.Background(), &agentpb.EnrollRequest{CsrDer: mustCSR(t), JoinToken: "builder-token", NodeName: "laptop"}); err == nil {
		t.Fatal("Enroll() error = nil, want failure")
	}
	if len(st.nodes) != 0 {
		t.Fatalf("node rows = %d, want none", len(st.nodes))
	}
}
