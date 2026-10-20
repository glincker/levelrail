package dataexport

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// platformDeployWindow is how far back the deploy duration and outcome
// gauges look; it matches the "last hour" wording in the metric help text.
const platformDeployWindow = time.Hour

// PlatformSource reads control plane health numbers; *store.DB satisfies it.
type PlatformSource interface {
	ReadPlatformStats(ctx context.Context, now time.Time, window time.Duration) (store.PlatformStats, error)
}

// PlatformSeries turns a stats read into the fixed, unlabelled-by-resource
// self metric set. The only label is status on the deploy outcome gauge.
func PlatformSeries(prefix string, st store.PlatformStats, at time.Time) []Series {
	g := func(name, help string, v float64, labels ...Label) Series {
		return Series{Name: prefix + name, Help: help, Labels: labels, Value: v, Time: at}
	}
	return []Series{
		g("reconcile_lag_seconds", "Age of the stalest reconcile status, seconds.", st.ReconcileLagSeconds),
		g("deploy_queue_depth", "Deploys waiting in the queue.", float64(st.DeploysQueued)),
		g("deploys_running", "Deploys currently running.", float64(st.DeploysRunning)),
		g("deploy_last_duration_seconds", "Duration of the most recent finished deploy in the last hour, seconds.", st.DeployLastSeconds),
		g("deploy_avg_duration_seconds", "Mean duration of deploys finished in the last hour, seconds.", st.DeployAvgSeconds),
		g("deploys_finished", "Deploys finished in the last hour by outcome.", float64(st.DeploysSucceeded), Label{"status", "succeeded"}),
		g("deploys_finished", "Deploys finished in the last hour by outcome.", float64(st.DeploysFailed), Label{"status", "failed"}),
		g("agent_connections", "Nodes whose agent is currently online.", float64(st.NodesByStatus[store.NodeStatusOnline])),
	}
}
