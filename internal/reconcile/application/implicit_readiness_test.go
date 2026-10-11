package application

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func listeningPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func noProbeDesired(port int) *store.DesiredService {
	return &store.DesiredService{Name: "web", Image: "img:v2", Port: port}
}

func TestDeployReadiness_NoProbeFallsBackToTCP(t *testing.T) {
	tests := []struct {
		name     string
		hostPort func(t *testing.T) int
		port     int
		wantOK   bool
		wantInfo string
	}{
		{name: "port never listens fails the deploy", hostPort: closedPort, port: 3000, wantOK: false, wantInfo: "TCP connection on port 3000"},
		{name: "listening port passes", hostPort: listeningPort, port: 3000, wantOK: true, wantInfo: "TCP connection on port 3000"},
		{name: "no port and no probe keeps container-running readiness", hostPort: closedPort, port: 0, wantOK: true, wantInfo: "no port or probe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(tt.hostPort(t))
			oldName := ContainerName("web", "img:v1", "")
			rt.seed(oldName, true)
			desired := noProbeDesired(tt.port)
			target := ContainerName("web", desired.Image, "")
			rt.setExitState(target, &docker.ExitState{Running: true})
			rec := &fakeGateRecorder{}
			opts := append(gateOptions(rec, 1, 0, 0), WithReadyBudget(400*time.Millisecond))

			c := New("web", &fakeStore{svc: desired}, rt, opts...)
			result, err := c.Reconcile(context.Background())

			if tt.wantOK {
				if err != nil {
					t.Fatalf("Reconcile() error = %v, want success", err)
				}
			} else {
				if err == nil {
					t.Fatal("Reconcile() error = nil, want the readiness failure")
				}
				if cond := conditionOf(t, result); cond.Status != reconcile.ConditionFalse || cond.Reason != deploysafety.ReasonReadinessFailed {
					t.Errorf("condition = %+v, want False/%s", cond, deploysafety.ReasonReadinessFailed)
				}
				old, _ := rt.InspectByName(context.Background(), oldName)
				if old == nil || !old.Running {
					t.Errorf("previous release = %+v, want it still running", old)
				}
				if rec.has(deploysafety.PhaseCutover, deploysafety.StatusPassed) {
					t.Errorf("events = %+v, want no cutover", rec.events)
				}
			}
			if !hasInfoContaining(rec, tt.wantInfo) {
				t.Errorf("no info gate event containing %q: %+v", tt.wantInfo, rec.events)
			}
		})
	}
}

func hasInfoContaining(rec *fakeGateRecorder, sub string) bool {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, e := range rec.events {
		if e.Status == deploysafety.StatusInfo && strings.Contains(e.Message, sub) {
			return true
		}
	}
	return false
}

func TestDeployReadiness_ExplicitProbeWinsOverTCP(t *testing.T) {
	desired := gateDesired()
	c := New("web", &fakeStore{svc: desired}, newFakeRuntime(1), gateOptions(&fakeGateRecorder{}, 1, 0, 0)...)
	got := c.deployReadinessProbeFor(desired)
	if got == nil || got.TCP || got.Path != "/healthz" {
		t.Fatalf("deployReadinessProbeFor() = %+v, want the explicit HTTP probe", got)
	}
}
