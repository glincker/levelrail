package main

import (
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuf struct {
	mu sync.Mutex
	sb strings.Builder
}

func (b *lockedBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.Write(p)
}

func (b *lockedBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.String()
}

func TestProgressPrintsLineAfterQuietInterval(t *testing.T) {
	var buf lockedBuf
	_, stop := startDeployProgress(&buf, "uploading", 20*time.Millisecond)
	defer stop()
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(buf.String(), "still uploading") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(buf.String(), "still uploading, ") || !strings.Contains(buf.String(), "elapsed") {
		t.Fatalf("want a progress line with the step and elapsed time, got %q", buf.String())
	}
}

func TestProgressIntervalFromEnv(t *testing.T) {
	env := func(v string) func(string) (string, bool) {
		return func(k string) (string, bool) { return v, k == envDeployProgressInterval && v != "" }
	}
	if got := deployProgressInterval(env("45s")); got != 45*time.Second {
		t.Fatalf("env interval = %s, want 45s", got)
	}
	if got := deployProgressInterval(env("junk")); got != defaultDeployProgressInterval {
		t.Fatalf("bad env interval = %s, want the default", got)
	}
}
