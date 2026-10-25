package application

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// A never-ready desired release must not leave every older previous release running.
func TestPruneSupersededPreviousKeepsNewestRunningOnly(t *testing.T) {
	now := time.Now()
	rt := newFakeRuntime(0)
	c := New("who", &fakeStore{}, rt)
	want := ContainerName("who", "docker.io/traefik/whoami:v1.10.1", "")
	v1 := ContainerName("who", "docker.io/traefik/whoami:v1.10.0", "")
	v0 := ContainerName("who", "docker.io/traefik/whoami@sha256:0ad2", "")
	rt.containers[want] = &docker.ContainerState{ID: "new", Name: want, Running: false, Created: now}
	rt.containers[v1] = &docker.ContainerState{ID: "v1", Name: v1, Running: true, Created: now.Add(-30 * time.Minute)}
	rt.containers[v0] = &docker.ContainerState{ID: "v0", Name: v0, Running: true, Created: now.Add(-60 * time.Minute)}

	c.pruneSupersededPrevious(context.Background(), []string{want})
	c.pruneSupersededPrevious(context.Background(), []string{want}) // idempotent

	if _, ok := rt.containers[v0]; ok {
		t.Fatal("older previous release was not removed")
	}
	if cs, ok := rt.containers[v1]; !ok || !cs.Running {
		t.Fatal("newest running previous release must keep serving")
	}
	if _, ok := rt.containers[want]; !ok {
		t.Fatal("desired container must not be touched")
	}
}

// With nothing running, the newest stopped previous release stays so it can be revived.
func TestPruneSupersededPreviousKeepsNewestStoppedWhenNoneRunning(t *testing.T) {
	now := time.Now()
	rt := newFakeRuntime(0)
	c := New("who", &fakeStore{}, rt)
	want := ContainerName("who", "docker.io/traefik/whoami:v1.10.1", "")
	v1 := ContainerName("who", "docker.io/traefik/whoami:v1.10.0", "")
	v0 := ContainerName("who", "docker.io/traefik/whoami@sha256:0ad2", "")
	rt.containers[want] = &docker.ContainerState{ID: "new", Name: want, Running: false, Created: now}
	rt.containers[v1] = &docker.ContainerState{ID: "v1", Name: v1, Running: false, Created: now.Add(-30 * time.Minute)}
	rt.containers[v0] = &docker.ContainerState{ID: "v0", Name: v0, Running: false, Created: now.Add(-60 * time.Minute)}

	c.pruneSupersededPrevious(context.Background(), []string{want})

	if _, ok := rt.containers[v1]; !ok {
		t.Fatal("newest stopped previous release must stay revivable")
	}
	if _, ok := rt.containers[v0]; ok {
		t.Fatal("older stopped release was not removed")
	}
}
