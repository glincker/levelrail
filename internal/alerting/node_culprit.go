package alerting

import (
	"context"
	"fmt"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

// nodeCulpritSuffix names the placed app using the most of the metric on a node, so a node alert says
// who to look at. It is attribution only: nothing is limited or moved.
func nodeCulpritSuffix(ctx context.Context, services NodeServiceSource, metrics MetricsSource, kind Kind, node store.Node, from, now time.Time) string {
	if services == nil || metrics == nil {
		return ""
	}
	placed, err := services.ListDesiredServicesByNode(ctx, node.ID)
	if err != nil || len(placed) < 2 {
		return ""
	}
	metric, unit := nodeResourceUsageMemoryMetric, "memory"
	if kind == KindNodeCPU {
		metric, unit = nodeResourceUsageCPUMetric, "CPU"
	}
	var top string
	var topVal, sum float64
	for _, svc := range placed {
		samples, qerr := metrics.QueryMetrics(ctx, "service:"+svc.Name, metric, from, now)
		if qerr != nil || len(samples) == 0 {
			continue
		}
		v := samples[len(samples)-1].Value
		sum += v
		if v > topVal {
			top, topVal = svc.Name, v
		}
	}
	if top == "" || sum <= 0 {
		return ""
	}
	return fmt.Sprintf("; largest %s user: %s (%.0f%% of placed apps)", unit, top, topVal/sum*100)
}
