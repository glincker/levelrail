package main

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/cloudmeta"
	"os"
)

// reportLocalOnboarding stores one probe result for the local node. The local node runs
// in-process with no agent session, so nothing else ever fills its preflight report.
func reportLocalOnboarding(ctx context.Context, sink agent.OnboardingSink, nodeID string, probe agent.OnboardingProbe, now time.Time) {
	if nodeID == "" {
		return
	}
	sink.SetNodeOnboarding(nodeID, probe(ctx), now)
}

// startLocalOnboarding feeds the local node's preflight report on the agent report interval,
// so `nodes doctor` runs the docker, disk, clock and port checks for it too.
func (s *bootState) startLocalOnboarding() {
	if s.client == nil || s.onboardingReports == nil {
		return
	}
	ctx := s.ctx
	probe := agent.NewOnboardingProbe(s.client, s.agentDataDir, s.meshCfg != nil, cloudmeta.SystemConfig())
	interval := agent.OnboardingIntervalFromEnv(os.LookupEnv)
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			// The id is re-read each tick: a fresh install writes it after boot.
			reportLocalOnboarding(ctx, s.onboardingReports, readLocalNodeID(s.agentDataDir), probe, time.Now())
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}
