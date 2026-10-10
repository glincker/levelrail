package exposure_test

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/exposure"
)

// Driven by a shell script against throwaway containers on a real Linux
// Docker host: EXPOSURE_LIVE_ACTION is audit, apply or remove.
func TestLiveExposure(t *testing.T) {
	action := os.Getenv("EXPOSURE_LIVE_ACTION")
	if testing.Short() || runtime.GOOS != "linux" || action == "" {
		t.Skip("set EXPOSURE_LIVE_ACTION on a Linux Docker host to run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	port, err := strconv.Atoi(os.Getenv("EXPOSURE_LIVE_PORT"))
	if err != nil {
		t.Fatalf("EXPOSURE_LIVE_PORT: %v", err)
	}
	allow := strings.Split(os.Getenv("EXPOSURE_LIVE_ALLOW"), ",")
	m := exposure.NewManager("acme:", []int{22, 80, 443, 8080, 9443})

	switch action {
	case "audit":
		printAudit(ctx, t, m)
	case "apply":
		plan, err := m.Plan(exposure.Restriction{Port: port, Allow: allow})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("plan:\n%s", strings.Join(plan.Commands, "\n"))
		changed, err := m.Apply(ctx, exposure.Restriction{Port: port, Allow: allow})
		t.Logf("apply changed=%v err=%v", changed, err)
		if err != nil {
			t.Fatal(err)
		}
		again, err := m.Apply(ctx, exposure.Restriction{Port: port, Allow: allow})
		t.Logf("second apply changed=%v err=%v", again, err)
		if err != nil || again {
			t.Fatalf("apply not idempotent: changed=%v err=%v", again, err)
		}
		printAudit(ctx, t, m)
	case "remove":
		if err := m.Remove(ctx, port, "tcp"); err != nil {
			t.Fatal(err)
		}
		printAudit(ctx, t, m)
	default:
		t.Fatalf("unknown action %q", action)
	}
}

func printAudit(ctx context.Context, t *testing.T, m *exposure.Manager) {
	t.Helper()
	chain := m.ReadChain(ctx)
	t.Logf("chain readable=%v reason=%q rules=%d", chain.Readable, chain.Reason, len(chain.Rules))
	for _, r := range chain.Rules {
		t.Logf("  rule: %s", r.Raw)
	}
	cli, err := docker.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	states, err := cli.ListByPrefix(ctx, "xp-")
	if err != nil {
		t.Fatal(err)
	}
	var cs []exposure.Container
	for _, s := range states {
		c := exposure.Container{ID: s.ID, Name: s.Name, Image: s.Image, Running: s.Running, Owner: exposure.Owner{Kind: exposure.OwnerUnmanaged}}
		for _, p := range s.Ports {
			c.Ports = append(c.Ports, exposure.PortBinding{HostIP: p.HostIP, HostPort: p.HostPort, ContainerPort: p.ContainerPort, Protocol: p.Protocol})
		}
		cs = append(cs, c)
	}
	for _, f := range exposure.Audit(cs, chain, m.Prefix()) {
		t.Logf("FINDING %s %d/%s binds=%v class=%s severity=%s managed=%v allowed=%v",
			f.Container, f.HostPort, f.Protocol, f.Binds, f.Class, f.Severity, f.Managed, f.AllowedSources)
	}
}
