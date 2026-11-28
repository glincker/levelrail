package agent

import (
	"testing"

	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/spec"
)

func cs(name, health string, labels map[string]string) docker.ContainerState {
	if labels == nil {
		labels = map[string]string{}
	}
	if _, ok := labels["unmanaged"]; ok {
		delete(labels, "unmanaged")
	} else {
		labels[spec.AppLabelKey] = "web"
	}
	return docker.ContainerState{
		Name: name, Running: true, Health: health, Labels: labels,
		Ports: []docker.PortBinding{{ContainerPort: 8080, HostPort: 20000 + len(name), HostIP: "127.0.0.1", Protocol: "tcp"}},
	}
}

func names(c []tunnelCandidate) []string {
	var out []string
	for _, x := range c {
		out = append(out, x.name)
	}
	return out
}

func TestTunnelSelectsActiveContainer(t *testing.T) {
	old := cs("web-aaaaaaaa", "healthy", nil)
	young := cs("web-bbbbbbbb", "starting", map[string]string{spec.SupersedesLabelKey: "web-aaaaaaaa"})
	// New release still starting: the old one keeps the traffic.
	if got := names(activeTunnelCandidates([]docker.ContainerState{old, young}, "web", 8080)); len(got) != 1 || got[0] != "web-aaaaaaaa" {
		t.Fatalf("starting successor: %v", got)
	}
	// Healthy successor: the superseded release is skipped.
	young.Health = "healthy"
	if got := names(activeTunnelCandidates([]docker.ContainerState{old, young}, "web", 8080)); len(got) != 1 || got[0] != "web-bbbbbbbb" {
		t.Fatalf("healthy successor: %v", got)
	}
	// Replicas share a release and all stay candidates.
	rep := cs("web-bbbbbbbb-r1", "healthy", map[string]string{spec.SupersedesLabelKey: "web-aaaaaaaa"})
	if got := names(activeTunnelCandidates([]docker.ContainerState{old, young, rep}, "web", 8080)); len(got) != 2 {
		t.Fatalf("replicas: %v", got)
	}
}

func TestTunnelRequiresManagedLabelAndRoundRobins(t *testing.T) {
	foreign := cs("web-aaaaaaaa", "", map[string]string{"unmanaged": "1"})
	if got := activeTunnelCandidates([]docker.ContainerState{foreign}, "web", 8080); len(got) != 0 {
		t.Fatalf("unmanaged container selected: %v", got)
	}
	unknown := cs("web-aaaaaaaa", "", nil)
	unknown.LabelsUnknown = true
	if got := activeTunnelCandidates([]docker.ContainerState{unknown}, "web", 8080); len(got) != 0 {
		t.Fatal("container with unknown labels selected")
	}
	a, b := cs("web-aaaaaaaa", "", nil), cs("web-aaaaaaaa-r1", "", nil)
	e := newTunnelEnv(t, 1)
	seen := map[int]bool{}
	for range 4 {
		p, reason := resolveTunnelPort(e.ctx, e.m, &tunnelRuntime{list: []docker.ContainerState{a, b}}, "web", 8080)
		if reason != "" {
			t.Fatal(reason)
		}
		seen[p] = true
	}
	if len(seen) != 2 {
		t.Fatalf("replicas not rotated: %v", seen)
	}
}
