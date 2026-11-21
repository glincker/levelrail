package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/cloudmeta"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/nodepreflight"
)

const doctorTimeout = 20 * time.Second

// runDoctor measures this host with the same probe the agent reports to the
// control plane, so the local result matches what the dashboard shows. The
// control plane and clock checks need the other side, so they are left out.
func runDoctor(out io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), doctorTimeout)
	defer cancel()
	client, err := docker.NewClient()
	if err != nil {
		_, _ = fmt.Fprintf(out, "docker: cannot create a client: %v\n", err)
		return 1
	}
	defer func() { _ = client.Close() }()
	report := agent.NewOnboardingProbe(client, agentDataDir(), false, cloudmeta.SystemConfig())(ctx)
	checks := localDoctorChecks(nodepreflight.Evaluate(report.Facts, time.Now(), nodepreflight.ConfigFromEnv(os.LookupEnv)))
	for _, c := range checks {
		_, _ = fmt.Fprintf(out, "%-6s %-14s %s\n", c.Status, c.ID, c.Detail)
		if c.Fix != "" {
			_, _ = fmt.Fprintf(out, "       fix: %s\n", c.Fix)
			if c.FixCommand != "" {
				_, _ = fmt.Fprintf(out, "       run: %s\n", c.FixCommand)
			}
		}
	}
	if nodepreflight.Summarize(checks).Fail > 0 {
		return 1
	}
	return 0
}

func localDoctorChecks(all []nodepreflight.Check) []nodepreflight.Check {
	out := make([]nodepreflight.Check, 0, len(all))
	for _, c := range all {
		if c.ID == nodepreflight.CheckControlPlane || c.ID == nodepreflight.CheckClock {
			continue
		}
		out = append(out, c)
	}
	return out
}
