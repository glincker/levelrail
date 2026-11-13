package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type volumeProbeRuntime struct {
	*fakeRuntime
	access   []docker.VolumeAccess
	err      error
	probed   int
	gotPaths []string
}

func (r *volumeProbeRuntime) ProbeVolumeAccess(_ context.Context, _ string, vols []docker.VolumeMount) ([]docker.VolumeAccess, error) {
	r.probed++
	for _, v := range vols {
		r.gotPaths = append(r.gotPaths, v.ContainerPath)
	}
	return r.access, r.err
}

func volumeConditionOf(result reconcile.Result) (reconcile.Condition, bool) {
	for _, c := range result.Conditions {
		if c.Type == ConditionTypeVolumesWritable {
			return c, true
		}
	}
	return reconcile.Condition{}, false
}

func cacheVolumeService() *store.DesiredService {
	return &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Volumes: []store.ServiceVolume{
			{Name: "app-web-next-cache", ContainerPath: "/app/.next/cache"},
		},
	}
}

func TestReconcile_VolumeWriteProbe(t *testing.T) {
	unwritable := docker.VolumeAccess{Volume: "app-web-next-cache", Path: "/app/.next/cache", User: "nextjs", UID: 1001, GID: 1001, Mode: 0o755}
	tests := []struct {
		name       string
		access     []docker.VolumeAccess
		err        error
		wantStatus reconcile.ConditionStatus
		wantReason string
		wantMsg    []string
	}{
		{name: "writable", access: []docker.VolumeAccess{{Path: "/app/.next/cache", UID: 1001, Writable: true}}, wantStatus: reconcile.ConditionTrue, wantReason: ReasonVolumesWritable},
		{
			name: "root-owned cache under a non-root user", access: []docker.VolumeAccess{unwritable},
			wantStatus: reconcile.ConditionFalse, wantReason: ReasonVolumeNotWritable,
			wantMsg: []string{"volume /app/.next/cache is not writable by user nextjs (uid 1001)", "owned by 0:0 with mode 755", "fix: chown the volume root to 1001:1001", "chown 1001:1001 /app/.next/cache before USER"},
		},
		{name: "user cannot be resolved", err: errors.New("resolve user: not in image /etc/passwd"), wantStatus: reconcile.ConditionUnknown, wantReason: ReasonVolumesUnverified, wantMsg: []string{"not in image /etc/passwd"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &volumeProbeRuntime{fakeRuntime: newFakeRuntime(0), access: tt.access, err: tt.err}
			c := New("web", &fakeStore{svc: cacheVolumeService()}, rt)
			result, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			cond, ok := volumeConditionOf(result)
			if !ok || cond.Status != tt.wantStatus || cond.Reason != tt.wantReason {
				t.Fatalf("volume condition = %+v (present %v), want %s/%s", cond, ok, tt.wantStatus, tt.wantReason)
			}
			for _, want := range tt.wantMsg {
				if !strings.Contains(cond.Message, want) {
					t.Errorf("message %q lacks %q", cond.Message, want)
				}
			}
			if len(rt.gotPaths) != 1 || rt.gotPaths[0] != "/app/.next/cache" {
				t.Errorf("probed paths = %v, want the volume mount", rt.gotPaths)
			}

			// A later pass with nothing to start reports the same result without probing again.
			again, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("second Reconcile() error = %v", err)
			}
			if cond2, ok := volumeConditionOf(again); !ok || cond2 != cond {
				t.Errorf("second pass condition = %+v, want %+v", cond2, cond)
			}
			if rt.probed != 1 {
				t.Errorf("probed %d times, want once per start", rt.probed)
			}
		})
	}
}

func TestReconcile_VolumeWriteProbe_UnsupportedRuntimeIsUnverified(t *testing.T) {
	c := New("web", &fakeStore{svc: cacheVolumeService()}, newFakeRuntime(0))
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond, ok := volumeConditionOf(result)
	if !ok || cond.Status != reconcile.ConditionUnknown || cond.Reason != ReasonVolumesUnverified || !strings.Contains(cond.Message, "not verified") {
		t.Errorf("volume condition = %+v (present %v), want Unknown/Unverified with a reason", cond, ok)
	}
}

func TestReconcile_VolumeWriteProbe_OffOrNoVolumes(t *testing.T) {
	t.Run("env off", func(t *testing.T) {
		t.Setenv(EnvVolumeWriteProbe, "off")
		rt := &volumeProbeRuntime{fakeRuntime: newFakeRuntime(0)}
		result, err := New("web", &fakeStore{svc: cacheVolumeService()}, rt).Reconcile(context.Background())
		if err != nil {
			t.Fatalf("Reconcile() error = %v", err)
		}
		if _, ok := volumeConditionOf(result); ok || rt.probed != 0 {
			t.Errorf("probe ran %d times or reported a condition with %s=off", rt.probed, EnvVolumeWriteProbe)
		}
	})
	t.Run("no writable volumes", func(t *testing.T) {
		rt := &volumeProbeRuntime{fakeRuntime: newFakeRuntime(0)}
		svc := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
		result, err := New("web", &fakeStore{svc: svc}, rt).Reconcile(context.Background())
		if err != nil {
			t.Fatalf("Reconcile() error = %v", err)
		}
		if _, ok := volumeConditionOf(result); ok || rt.probed != 0 {
			t.Errorf("probe ran %d times or reported a condition for an app without volumes", rt.probed)
		}
	})
}

func TestVolumeWriteMemo_WorstAndPrune(t *testing.T) {
	var m volumeWriteMemo
	m.set("web-a", volumeCondition(reconcile.ConditionTrue, ReasonVolumesWritable, ""))
	m.set("web-b", volumeCondition(reconcile.ConditionFalse, ReasonVolumeNotWritable, "x"))
	m.set("web-old", volumeCondition(reconcile.ConditionUnknown, ReasonVolumesUnverified, "y"))
	if got, ok := m.worst([]string{"web-a", "web-b"}); !ok || got.Status != reconcile.ConditionFalse {
		t.Errorf("worst = %+v, want the False result", got)
	}
	if _, ok := m.byContainer["web-old"]; ok {
		t.Error("a container that is no longer a target was kept")
	}
	if _, ok := m.worst(nil); ok {
		t.Error("worst with no targets reported a result")
	}
}
