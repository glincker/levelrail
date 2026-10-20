package attention

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestBuild_NodeCertAndAgentVersion(t *testing.T) {
	t.Parallel()
	na := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	node := func(state string, outdated bool) apiclient.NodeResource {
		return apiclient.NodeResource{
			Name: "a", Status: "online",
			Cert:  &apiclient.NodeCertResource{State: state, NotAfter: &na},
			Agent: &apiclient.NodeAgentResource{Version: "v0.1.0", MinVersion: "v0.2.0", Outdated: outdated},
		}
	}
	tests := []struct {
		name         string
		node         apiclient.NodeResource
		wantSeverity string
		wantKind     string
	}{
		{"healthy", node("ok", false), "", ""},
		{"expiring", node("expiring", false), Warning, "node_cert"},
		{"critical", node("critical", false), Critical, "node_cert"},
		{"expired", node("expired", false), Critical, "node_cert"},
		{"revoked", node("revoked", false), Warning, "node_cert"},
		{"outdated agent", node("ok", true), Warning, "node_agent"},
		{"no cert data from an older control plane", apiclient.NodeResource{Name: "a", Status: "online"}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Build(Input{Nodes: []apiclient.NodeResource{tt.node}})
			if tt.wantKind == "" {
				if len(got) != 0 {
					t.Fatalf("got %+v, want none", got)
				}
				return
			}
			if len(got) != 1 || got[0].Severity != tt.wantSeverity || got[0].Kind != tt.wantKind || got[0].Subject != "a" || got[0].Detail == "" {
				t.Fatalf("got %+v, want one %s %s item for a", got, tt.wantSeverity, tt.wantKind)
			}
		})
	}
}
