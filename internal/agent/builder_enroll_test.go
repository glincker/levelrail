package agent

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

type trustRecordingStore struct {
	*fakeEnrollStore
	tiers map[string]string
}

func (s *trustRecordingStore) SetNodeBuildTrust(_ context.Context, id, tier string) error {
	s.tiers[id] = tier
	return nil
}

func TestPersonalBuilderEnrolsBuildOnlyAndStaysOffProduction(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	st := &trustRecordingStore{fakeEnrollStore: newFakeEnrollStore(), tiers: map[string]string{}}
	seedJoinToken(st.fakeEnrollStore, "builder-token", time.Hour)
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
	if st.tiers[n.ID] != store.BuildTrustPersonal {
		t.Fatalf("tier = %q, want personal", st.tiers[n.ID])
	}

	info := []build.NodeInfo{{ID: n.ID, Name: n.Name, AcceptsBuildWorkloads: true, Online: true, Trust: st.tiers[n.ID]}}
	if d := build.ChooseBuildNode(info, build.Query{App: "web", EnvKind: "production"}, build.Policy{Now: time.Now()}); d.Node != "" {
		t.Fatalf("personal builder chosen for production: %+v", d)
	}
}
