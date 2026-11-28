package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/awsx"
)

func TestDeepProbesPassDeniedAndTimeout(t *testing.T) {
	probes := []deepProbe{
		{"deep.aws.ok", "Fine", "ec2:DescribeInstances", func(context.Context) error { return nil }},
		{"deep.aws.denied", "Denied", "ec2:DescribeInstances", func(context.Context) error {
			return &awsx.Error{Code: "UnauthorizedOperation", Class: awsx.ClassAuth}
		}},
		{"deep.aws.throttled", "Throttled", "rds:DescribeDBInstances", func(context.Context) error {
			return &awsx.Error{Code: "Throttling", Class: awsx.ClassThrottle}
		}},
		{"deep.aws.broken", "Broken", "acm:ListCertificates", func(context.Context) error {
			return &awsx.Error{Code: "InternalFailure", Class: awsx.ClassServer}
		}},
		{"deep.aws.slow", "Slow", "ecr:DescribeRepositories", func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}},
	}
	steps := runDeepProbes(context.Background(), probes, 2, 50*time.Millisecond)
	want := []string{doctorOK, doctorWarn, doctorWarn, doctorFail, doctorFail}
	for i, s := range steps {
		if s.ID != probes[i].ID || s.Status != want[i] {
			t.Fatalf("step %d = %+v, want status %s", i, s, want[i])
		}
	}
	if !strings.Contains(steps[1].Fix, "ec2:DescribeInstances") || !strings.Contains(steps[1].Fix, "inventory") {
		t.Fatalf("denied fix must name the action and tier: %q", steps[1].Fix)
	}
	if !strings.Contains(steps[4].Detail, "timed out") || !strings.Contains(steps[4].Fix, "deep_timeout_seconds") {
		t.Fatalf("timeout step = %+v", steps[4])
	}
	if got := deepSummary(steps); got != "1 pass, 2 warn, 2 fail" {
		t.Fatalf("summary = %q", got)
	}
}
