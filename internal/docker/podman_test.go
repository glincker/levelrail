package docker

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	dockertypes "github.com/docker/docker/api/types"
)

func TestRuntimeKindLabel(t *testing.T) {
	if RuntimeKindPodman.Label() != "Podman" || RuntimeKindDocker.Label() != "Docker" || RuntimeKindUnknown.Label() == "" {
		t.Fatal("unexpected engine labels")
	}
}

func TestPoolFromEnv_PodmanDefaultAvoidsNetavarkRange(t *testing.T) {
	none := func(string) (string, bool) { return "", false }
	p := poolFromEnv(none, RuntimeKindPodman)
	if p == nil || p.Base.String() != DefaultPodmanNetworkPool {
		t.Fatalf("podman pool = %v", p)
	}
	if got, ok := p.Allocate(nil); !ok || got.Overlaps(podmanDefaultSubnet) {
		t.Fatalf("allocated %v", got)
	}
	d := poolFromEnv(none, RuntimeKindDocker)
	if d == nil || d.Base.String() != DefaultNetworkPool {
		t.Fatalf("docker pool = %v", d)
	}
}

func TestPoolFromEnv_EnvOverrides(t *testing.T) {
	env := map[string]string{EnvNetworkPoolPodman: "10.50.0.0/16"}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	if p := poolFromEnv(lookup, RuntimeKindPodman); p == nil || p.Base.String() != "10.50.0.0/16" {
		t.Fatalf("podman env pool = %v", p)
	}
	env[EnvNetworkPool] = "10.60.0.0/16"
	if p := poolFromEnv(lookup, RuntimeKindPodman); p == nil || p.Base.String() != "10.60.0.0/16" {
		t.Fatalf("explicit pool must win, got %v", p)
	}
}

func TestClient_RemovePodmanStopsThenRemoves(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	c := fakeDockerClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/stop"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete:
			if r.URL.Query().Get("force") != "1" && r.URL.Query().Get("force") != "true" {
				t.Errorf("remove without force: %s", r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			jsonOK(w, dockertypes.Version{Platform: struct{ Name string }{Name: "Podman Engine"}})
		}
	})
	if err := c.Remove(context.Background(), "abc", true); err != nil {
		t.Fatalf("Remove() = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	stop, del := -1, -1
	for i, call := range calls {
		if strings.HasSuffix(call, "/containers/abc/stop") {
			stop = i
		}
		if strings.HasPrefix(call, "DELETE") {
			del = i
		}
	}
	if stop < 0 || del < 0 || stop > del {
		t.Fatalf("want stop before delete, calls = %v", calls)
	}
}

func TestClient_RemovePodmanToleratesStoppedAndGone(t *testing.T) {
	c := fakeDockerClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/stop"):
			w.WriteHeader(http.StatusNotModified)
		case r.Method == http.MethodDelete:
			jsonError(w, http.StatusNotFound, "no such container")
		default:
			jsonOK(w, dockertypes.Version{Platform: struct{ Name string }{Name: "Podman Engine"}})
		}
	})
	c.runtime.Kind = RuntimeKindPodman
	if err := c.Remove(context.Background(), "abc", true); err != nil {
		t.Fatalf("Remove() = %v", err)
	}
}

func TestClient_RemoveDockerSkipsStop(t *testing.T) {
	stopped := false
	c := fakeDockerClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/stop"):
			stopped = true
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			jsonOK(w, dockertypes.Version{Platform: struct{ Name string }{Name: "Docker Engine - Community"}})
		}
	})
	if err := c.Remove(context.Background(), "abc", true); err != nil {
		t.Fatalf("Remove() = %v", err)
	}
	if stopped {
		t.Fatal("docker path must keep the single force-remove")
	}
}

func TestClient_StopTimeoutFromEnv(t *testing.T) {
	t.Setenv(envPodmanStopTimeout, "3s")
	if got := envDuration(envPodmanStopTimeout, defaultPodmanStopTimeout); got != 3*time.Second {
		t.Fatalf("timeout = %v", got)
	}
}
