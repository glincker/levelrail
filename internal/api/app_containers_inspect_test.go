package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/secretmask"
)

// inspectingRuntime is a remote node whose agent may or may not answer live inspects.
type inspectingRuntime struct {
	*fakeExecAppRuntime
	details []docker.ContainerDetail
	err     error
	calls   int
	query   docker.DetailQuery
}

func (f *inspectingRuntime) InspectContainerDetails(_ context.Context, q docker.DetailQuery) ([]docker.ContainerDetail, error) {
	f.calls++
	f.query = q
	out := make([]docker.ContainerDetail, 0, len(f.details))
	for _, d := range f.details {
		d.InstanceID = q.LabelValue
		out = append(out, d)
	}
	return out, f.err
}

const healthSecret = "s3cr3t-health-probe-token"

func remoteInspectFixture(t *testing.T, err error, withDetail bool) (*inspectingRuntime, []appContainerResource, string) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	name := seedContainersApp(t, db, "node-2")
	fake := &inspectingRuntime{
		fakeExecAppRuntime: &fakeExecAppRuntime{listByPrefixResult: []docker.ContainerState{{ID: "abc", Name: name, Image: "img:v1", Running: true}}},
		err:                err,
	}
	if withDetail {
		fake.details = []docker.ContainerDetail{{
			ID: "abc", Name: name, State: "running", RepoDigests: []string{"img@sha256:feed"}, RestartCount: 5, StartedAt: time.Now().Add(-time.Minute),
			Health:   &docker.ContainerHealthDetail{Status: "unhealthy", FailingStreak: 3, LastOutput: "connection refused token=" + healthSecret},
			Networks: []docker.ContainerNetworkDetail{{Name: "lr", IP: "10.1.0.4", Aliases: []string{"web"}}},
			Mounts: []docker.ContainerMountDetail{{Type: "bind", Source: "/srv/injected", Target: "/app", ReadOnly: true},
				{Type: "volume", Source: "web-data", Target: "/data"}},
			NanoCPUs: 2e9, MemoryBytes: 1 << 30,
		}, {ID: "sib", Name: "web-0-" + name[len("web-"):], State: "running"}}
	}
	rt.execRuntime = func(string) (docker.Runtime, error) { return fake, nil }
	rec, got := getAppContainers(t, rt, db)
	if rec.Code != 200 || len(got) != 1 {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "hunter2") || strings.Contains(rec.Body.String(), healthSecret) {
		t.Fatalf("response leaks env: %s", rec.Body.String())
	}
	return fake, got, strings.Join(got[0].Unavailable, ",")
}

func TestAppContainers_LiveInspectFillsRemoteFields(t *testing.T) {
	t.Parallel()
	secretmask.Add(healthSecret)
	fake, got, unavailable := remoteInspectFixture(t, nil, true)
	c := got[0]
	if len(fake.query.IDs) != 1 || fake.query.IDs[0] != "abc" {
		t.Errorf("query ids = %v, want only the owned replica", fake.query.IDs)
	}
	if c.Health == nil || c.Health.FailingStreak != 3 || c.RestartCount == nil || *c.RestartCount != 5 || c.StartedAt == nil {
		t.Errorf("health/restart = %+v %v %v", c.Health, c.RestartCount, c.StartedAt)
	}
	if c.ImageDigest != "img@sha256:feed" || len(c.Networks) != 1 || c.Networks[0].Aliases[0] != "web" {
		t.Errorf("digest/networks = %q %+v", c.ImageDigest, c.Networks)
	}
	if len(c.Mounts) != 2 || c.Mounts[0].Source != "(managed)" || c.Mounts[1].Source != "web-data" || c.Limits.CPU != 2 || c.Limits.MemoryBytes != 1<<30 {
		t.Errorf("live mounts/limits = %+v %+v", c.Mounts, c.Limits)
	}
	for _, field := range []string{"health", "restart_count", "started_at", "networks", "image_digest", "live_inspect"} {
		if strings.Contains(unavailable, field) {
			t.Errorf("unavailable = %q still has %q", unavailable, field)
		}
	}
}

func TestAppContainers_LiveInspectCapabilityAbsentKeepsMarkers(t *testing.T) {
	t.Parallel()
	_, got, unavailable := remoteInspectFixture(t, docker.ErrDetailUnsupported, false)
	if got[0].Health != nil || got[0].RestartCount != nil {
		t.Errorf("container = %+v", got[0])
	}
	for _, want := range []string{"health", "restart_count", "started_at", "networks", "image_digest"} {
		if !strings.Contains(unavailable, want) {
			t.Errorf("unavailable = %q, missing %q", unavailable, want)
		}
	}
	if strings.Contains(unavailable, "live_inspect") {
		t.Errorf("old agent got a live_inspect marker: %q", unavailable)
	}
}

func TestAppContainers_LiveInspectTimeoutReturnsPartial(t *testing.T) {
	t.Parallel()
	fake, got, unavailable := remoteInspectFixture(t, context.DeadlineExceeded, false)
	if fake.calls != 1 || got[0].Name == "" || got[0].State != "running" {
		t.Fatalf("calls = %d container = %+v", fake.calls, got[0])
	}
	if !strings.Contains(unavailable, "live_inspect") || !strings.Contains(unavailable, "health") {
		t.Errorf("unavailable = %q", unavailable)
	}
}
