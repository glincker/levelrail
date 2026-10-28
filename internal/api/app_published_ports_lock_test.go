package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// blockingResolver parks every image whose ref starts with "slow/" until released.
type blockingResolver struct{ release chan struct{} }

func (b blockingResolver) ResolveImage(ctx context.Context, ref string, _ *docker.RegistryAuth, _ bool) (docker.ResolvedImage, error) {
	if strings.HasPrefix(ref, "slow/") {
		select {
		case <-b.release:
		case <-ctx.Done():
		}
	}
	return docker.ResolvedImage{Ref: ref}, nil
}

func createPortApp(t *testing.T, rt *Router, cookie *http.Cookie, name, image, ports string) int {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q,"image":%q,"port":0,"published_ports":%s}`, name, image, ports)
	code, _ := quotaDo(t, rt, cookie, http.MethodPost, "/api/v1/apps", body)
	return code
}

func TestPublishedPortsLock_PortlessCreateDoesNotWait(t *testing.T) {
	rt, _, cookie, proj := quotaFixture(t)
	rt.publishedPortsMu.Lock()
	defer rt.publishedPortsMu.Unlock()
	done := make(chan int, 1)
	go func() {
		code, _ := createQuotaApp(t, rt, cookie, "plain", proj, "")
		done <- code
	}()
	select {
	case code := <-done:
		if code != http.StatusCreated {
			t.Fatalf("create = %d, want 201", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a create with no published ports waited on the published port lock")
	}
}

func TestPublishedPortsLock_SlowRegistryDoesNotStallPortCreates(t *testing.T) {
	stubUDPListeners(t)
	rt, _, cookie, _ := quotaFixture(t)
	res := blockingResolver{release: make(chan struct{})}
	rt.imageResolver = res
	slow := make(chan int, 1)
	go func() {
		slow <- createPortApp(t, rt, cookie, "slowapp", "slow/img:1", `[{"container_port":53,"host_port":5301,"protocol":"udp"}]`)
	}()
	time.Sleep(100 * time.Millisecond)
	fast := make(chan int, 1)
	go func() {
		fast <- createPortApp(t, rt, cookie, "fastapp", "fast/img:1", `[{"container_port":53,"host_port":5302,"protocol":"udp"}]`)
	}()
	select {
	case code := <-fast:
		if code != http.StatusCreated {
			t.Fatalf("fast create = %d, want 201", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a slow registry lookup held the published port lock")
	}
	close(res.release)
	if code := <-slow; code != http.StatusCreated {
		t.Fatalf("slow create = %d, want 201", code)
	}
}

func TestPublishedPortsLock_ConcurrentConflictingCreatesAdmitOne(t *testing.T) {
	stubUDPListeners(t)
	rt, _, cookie, _ := quotaFixture(t)
	const attempts = 10
	var wg sync.WaitGroup
	codes := make(chan int, attempts)
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- createPortApp(t, rt, cookie, fmt.Sprintf("dns%d", i), "levelrail/dns:1", `[{"container_port":53,"host_port":5353,"protocol":"udp"}]`)
		}()
	}
	wg.Wait()
	close(codes)
	created := 0
	for c := range codes {
		if c == http.StatusCreated {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created %d apps on one udp host port, want exactly 1", created)
	}
}
