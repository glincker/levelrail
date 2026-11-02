package build

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rpplan "github.com/railwayapp/railpack/core/plan"
)

func TestAutoMemoryCapMB(t *testing.T) {
	g := MemGuard{Percent: 60, ReserveMB: 512, NodeHeapPercent: 75}
	for _, c := range []struct{ host, want int }{
		{0, 0}, {512, 256}, {1024, 512}, {2048, 1228}, {4096, 2457}, {8192, 4915}, {65536, 39321},
	} {
		if got := AutoMemoryCapMB(c.host, g); got != c.want {
			t.Errorf("host %d: cap %d, want %d", c.host, got, c.want)
		}
	}
	if got := EffectiveMemoryMB(3000, 2048, g); got != 3000 {
		t.Errorf("explicit setting = %d", got)
	}
	if got := EffectiveMemoryMB(0, 2048, g); got != 1228 {
		t.Errorf("auto = %d", got)
	}
}

func TestMemGuardFromEnv(t *testing.T) {
	env := map[string]string{EnvBuildMemoryPercent: "50", EnvBuildMemoryReserve: "x", EnvBuildNodeHeapPct: "5"}
	g := MemGuardFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if g.Percent != 50 || g.ReserveMB != DefaultMemoryReserveMB || g.NodeHeapPercent != DefaultNodeHeapPercent {
		t.Fatalf("guard = %+v", g)
	}
}

func TestDetectBuildFrameworkAndWarning(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"next":"15.0.0"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := DetectBuildFramework(dir); got != FrameworkNextJS {
		t.Fatalf("framework = %s", got)
	}
	if LowMemoryWarning(FrameworkNextJS, 1024) == "" || LowMemoryWarning(FrameworkNextJS, 4096) != "" || LowMemoryWarning(FrameworkOther, 1024) != "" {
		t.Fatal("warning floors wrong")
	}
}

func TestApplyNodeHeapRespectsExistingNodeOptions(t *testing.T) {
	p := &rpplan.BuildPlan{Steps: []rpplan.Step{{Name: "install"}, {Name: "build"}, {Name: "other"}}}
	msg := applyNodeHeap(p, 900)
	if p.Steps[1].Variables["NODE_OPTIONS"] != "--max-old-space-size=900" || p.Steps[2].Variables["NODE_OPTIONS"] != "" || strings.Contains(msg, "unchanged") {
		t.Fatalf("plan = %+v msg = %s", p.Steps, msg)
	}
	q := &rpplan.BuildPlan{Steps: []rpplan.Step{{Name: "build", Variables: map[string]string{"NODE_OPTIONS": "--x"}}}}
	if msg := applyNodeHeap(q, 900); q.Steps[0].Variables["NODE_OPTIONS"] != "--x" || !strings.Contains(msg, "unchanged") {
		t.Fatalf("existing NODE_OPTIONS overwritten: %+v", q.Steps)
	}
}

type railpackSpy struct {
	fakeBuilder
	heap *int
	err  error
}

func (r railpackSpy) BuildRailpack(_ context.Context, req RailpackRequest, _ func(ProgressEvent)) (*Result, error) {
	*r.heap = req.NodeHeapMB
	return nil, r.err
}

func TestLimiterAppliesHeapAndExplainsOOM(t *testing.T) {
	heap := 0
	cg := &fakeCgroups{parent: "levelrail-builds/web"}
	inner := railpackSpy{heap: &heap, err: errors.New("process did not complete successfully: exit code: 137")}
	l := NewLimiter(inner, fixed(Limits{MemoryMB: 1000}), WithCgroupProvisioner(cg), WithMemoryGuard(MemGuard{NodeHeapPercent: 75}, func() int { return 2048 }))

	_, err := l.BuildRailpack(context.Background(), RailpackRequest{Tag: "reg/web:abc", SourceDir: t.TempDir()}, nil)
	if heap != 750 || cg.mem != 1000 {
		t.Fatalf("heap = %d, cgroup mem = %d", heap, cg.mem)
	}
	for _, want := range []string{"builds.memory_limit_mb", "1000 MiB", "2048 MiB", "OOMKilled", "builds-only node", "swap"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("message missing %q: %v", want, err)
		}
	}
}
