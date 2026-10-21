package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHungDaemonFailsFast(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)

	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))
	t.Setenv(envBootTimeout, "300ms")
	t.Setenv(envCallTimeout, "300ms")

	start := time.Now()
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = c.Close() }()
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("NewClient took %s against a hung daemon", d)
	}

	var wg sync.WaitGroup
	start = time.Now()
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errs[0] = c.BridgeGatewayIP(context.Background())
	}()
	go func() {
		defer wg.Done()
		_, errs[1] = c.ListContainersByLabel(context.Background(), "a=b")
	}()
	wg.Wait()
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("concurrent calls took %s, want about the call timeout", d)
	}
	for i, e := range errs {
		if e == nil {
			t.Fatalf("call %d returned nil error against a hung daemon", i)
		}
	}

	start = time.Now()
	if _, err := c.ListContainersByLabel(context.Background(), "a=b"); err == nil {
		t.Fatal("expected fail-fast error while breaker is open")
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("breaker did not fail fast: %s", d)
	}
}
