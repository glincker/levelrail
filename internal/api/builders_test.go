package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestBuilderViewJoinsNodeTrustAndHistory(t *testing.T) {
	now := time.Now()
	n := build.NodeInfo{ID: "n1", Name: "mac", Online: true, AcceptsBuildWorkloads: true, Trust: build.TrustPersonal, SlotsUsed: 1, SlotsTotal: 2, Arch: "linux/arm64", LastSeen: now}
	runs := []store.BuildRun{
		{App: "web", NodeID: "n1", Status: "success", DurationMS: 2000, StartedAt: now},
		{App: "api", NodeID: "n1", Status: "failed", DurationMS: 500, StartedAt: now.Add(-time.Minute)},
		{App: "web", NodeID: "n1", Status: "success", DurationMS: 4000, StartedAt: now.Add(-2 * time.Minute)},
		{App: "other", NodeID: "n2", Status: "success", DurationMS: 9000, StartedAt: now},
	}
	d := builderView(n, store.Node{AgentOS: "linux", AgentArch: "arm64", AgentVersion: "1.5.7"}, runs)
	if d.Tier != "personal" || d.OS != "linux" || d.Arch != "arm64" || d.Status != "ready" || d.ActiveBuilds != 1 {
		t.Fatalf("unexpected view %+v", d.builderResource)
	}
	if d.RecentBuilds != 3 || d.RecentFailed != 1 || d.AvgBuildMS != 3000 || len(d.Builds) != 3 {
		t.Fatalf("history wrong: %+v", d)
	}
}

func TestBuilderViewPausedAndPlatformFallback(t *testing.T) {
	d := builderView(build.NodeInfo{ID: "n1", Name: "box", Online: true, Draining: true, Arch: "linux/amd64"}, store.Node{}, nil)
	if !d.Paused || d.Status != "paused" || d.Tier != build.TrustServer || d.OS != "linux" || d.Arch != "amd64" || d.Builds == nil {
		t.Fatalf("unexpected %+v", d)
	}
}
