package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/nodecapacity"
	"github.com/GLINCKER/levelrail/internal/store"
)

// buildRunnerNodeSource lists the control plane and every build node with the facts routing and the runner view need.
func buildRunnerNodeSource(db *store.DB, agentRegistry *agent.Registry, reports *agent.HostReportStore) build.NodeSource {
	thresholds := nodecapacity.ThresholdsFromEnv(os.LookupEnv)
	return func(ctx context.Context) ([]build.NodeInfo, error) {
		nodes, err := db.ListNodes(ctx)
		if err != nil {
			return nil, fmt.Errorf("list nodes: %w", err)
		}
		labels, err := db.ListNodeLabels(ctx)
		if err != nil {
			return nil, fmt.Errorf("list node labels: %w", err)
		}
		infos := make([]build.NodeInfo, 0, len(nodes)+1)
		infos = append(infos, build.NodeInfo{
			Name: "control plane", Local: true, Online: true, AcceptsBuildWorkloads: true,
			Arch: build.HostPlatform(), CPUCores: runtime.NumCPU(), Pressure: build.LocalPressure(),
		})
		for _, n := range nodes {
			info := build.NodeInfo{
				ID: n.ID, Name: n.Name, AcceptsBuildWorkloads: n.AcceptsBuildWorkloads,
				Draining: !n.Schedulable, Labels: labels[n.ID], Pressure: build.PressureUnknown,
			}
			if _, err := agentRegistry.Get(n.ID); err == nil {
				info.Online = true
			}
			if n.LastSeenAt != nil {
				info.LastSeen = *n.LastSeenAt
			}
			if n.AgentOS != "" && n.AgentArch != "" {
				info.Arch = n.AgentOS + "/" + n.AgentArch
			}
			if reports != nil {
				if rep, ok := reports.Get(n.ID); ok {
					info.CPUCores = int(rep.CPUCores)
					info.Pressure = nodePressure(nodecapacity.Evaluate(nodecapacity.Sample{
						DiskTotal: rep.DiskTotalBytes, DiskFree: rep.DiskFreeBytes,
						InodesTotal: rep.DiskInodesTotal, InodesFree: rep.DiskInodesFree,
						MemTotal: rep.MemoryTotalBytes, MemAvail: rep.MemoryAvailableBytes,
						SwapTotal: rep.SwapTotalBytes, SwapFree: rep.SwapFreeBytes,
					}, thresholds).Worst)
				}
			}
			infos = append(infos, info)
		}
		return infos, nil
	}
}

func nodePressure(l nodecapacity.Level) string {
	switch l {
	case nodecapacity.LevelOK:
		return build.PressureOK
	case nodecapacity.LevelWarn:
		return build.PressureWarn
	case nodecapacity.LevelCritical:
		return build.PressureCritical
	}
	return build.PressureUnknown
}

const runnerAffinityHistory = 20

// buildRunnerQuery adds what is known about an app: its runner preference, where it last built and which architecture it runs on.
func buildRunnerQuery(db *store.DB) func(ctx context.Context, app string) build.Query {
	return func(ctx context.Context, app string) build.Query {
		q := build.Query{App: app}
		if app == "" {
			return q
		}
		if p, ok, err := db.GetBuildRunnerPref(ctx, app); err == nil && ok {
			if p.Pinned {
				q.Pinned = p.NodeID
			} else {
				q.Preferred = p.NodeID
			}
			q.Labels = parseRunnerLabels(p.Labels)
		}
		if runs, err := db.ListBuildRuns(ctx, app, runnerAffinityHistory); err == nil {
			for _, r := range runs {
				if r.Status == string(build.RunSuccess) && r.NodeID != "" {
					q.LastNode = r.NodeID
					break
				}
			}
		}
		q.TargetArch = targetArch(ctx, db, app)
		return q
	}
}

// targetArch is the architecture of the node the app is placed on, empty when unknown.
func targetArch(ctx context.Context, db *store.DB, app string) string {
	svc, err := db.GetDesiredService(ctx, app)
	if err != nil || svc == nil {
		return ""
	}
	if svc.NodeID == "" {
		return build.HostPlatform()
	}
	n, err := db.GetNode(ctx, svc.NodeID)
	if err != nil || n.AgentOS == "" || n.AgentArch == "" {
		return ""
	}
	return n.AgentOS + "/" + n.AgentArch
}

func parseRunnerLabels(in []string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for _, l := range in {
		k, v, _ := strings.Cut(l, "=")
		out[k] = v
	}
	return out
}
