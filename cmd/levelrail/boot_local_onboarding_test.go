package main

import (
	"context"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/agent"
	"github.com/glincker/levelrail/internal/nodepreflight"
)

func TestReportLocalOnboardingFillsStore(t *testing.T) {
	store := agent.NewOnboardingStore()
	probe := func(context.Context) agent.OnboardingReport {
		return agent.OnboardingReport{Facts: nodepreflight.Facts{DockerOK: true}}
	}
	now := time.Now()
	reportLocalOnboarding(context.Background(), store, "", probe, now)
	if _, ok := store.Get(""); ok {
		t.Fatal("empty node id must not be stored")
	}
	reportLocalOnboarding(context.Background(), store, "node-local", probe, now)
	got, ok := store.Get("node-local")
	if !ok || !got.Report.Facts.DockerOK || !got.ReceivedAt.Equal(now) {
		t.Fatalf("local report not stored: %+v ok=%v", got, ok)
	}
}
