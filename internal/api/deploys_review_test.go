package api

import (
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/deploysafety"
	"github.com/glincker/levelrail/internal/reconcile"
)

func TestImageUpdateReviewLine(t *testing.T) {
	got := imageUpdateReviewLine("ghcr.io/acme/web:1@sha256:aaaaaaaaaaaaaaaa", "sha256:bbbbbbbbbbbbbbbb", "ghcr.io/acme/web:1")
	for _, want := range []string{"sha256:aaaaaaaaaaaa to sha256:bbbbbbbbbbbb", "https://github.com/acme/web/pkgs/container/web"} {
		if !strings.Contains(got, want) {
			t.Errorf("line %q missing %q", got, want)
		}
	}
	if imageUpdateReviewLine("nginx:1@sha256:aa", "sha256:aa", "nginx:1") != "" {
		t.Error("unchanged digest must give no line")
	}
	if imageChangelogLink("nginx:1") != "https://hub.docker.com/_/nginx" {
		t.Error("official image link wrong")
	}
}

func TestCrashBackoffFrom(t *testing.T) {
	now := time.Now()
	msg := "crash loop (4 restarts), next restart in 30s"
	conds := []reconcile.Condition{{Reason: deploysafety.ReasonCrashLoopBackOff, Message: msg}}
	b := crashBackoffFrom(conds, now)
	if _, wait, ok := deploysafety.ParseCrashLoop(msg); ok {
		if b == nil || b.RetryInSeconds != int(wait.Seconds()) {
			t.Fatalf("backoff = %+v", b)
		}
	} else if b != nil {
		t.Fatal("unparsable message must give nil")
	}
	if crashBackoffFrom(nil, now) != nil {
		t.Error("no condition must give nil")
	}
}
