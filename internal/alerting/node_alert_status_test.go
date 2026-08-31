package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func healthyNodeAlertSamples(node store.Node, now time.Time) map[string]map[string][]telemetry.Sample {
	resourceID := "node:" + node.ID
	return map[string]map[string][]telemetry.Sample{
		resourceID: {
			telemetry.MetricOSSecurityPatchesAvailable: {{Timestamp: now, Value: 0}},
			telemetry.MetricDiskUsedBytes:              {{Timestamp: now, Value: 10}},
			telemetry.MetricDiskTotalBytes:             {{Timestamp: now, Value: 100}},
		},
	}
}

func TestCheckNodeAlertStatus_AllHealthy(t *testing.T) {
	now := time.Now()
	node := store.Node{ID: "n1", Name: "web-1"}
	metrics := &perMetricSource{samples: healthyNodeAlertSamples(node, now)}
	services := &fakeNodeServiceSource{}

	got := CheckNodeAlertStatus(context.Background(), node, services, metrics, 1, 90, 80, 4<<30, now, nil)
	if got.PatchStatus != NodeAlertOK {
		t.Errorf("PatchStatus = %v, want ok", got.PatchStatus)
	}
	if got.NodeDiskSpace != NodeAlertOK {
		t.Errorf("NodeDiskSpace = %v, want ok", got.NodeDiskSpace)
	}
	if got.NodeResourceUsage != NodeAlertUnknown {
		t.Errorf("NodeResourceUsage = %v, want unknown (no services placed on this node)", got.NodeResourceUsage)
	}
}

func TestCheckNodeAlertStatus_PatchFiring(t *testing.T) {
	now := time.Now()
	node := store.Node{ID: "n1", Name: "web-1"}
	samples := healthyNodeAlertSamples(node, now)
	samples["node:n1"][telemetry.MetricOSSecurityPatchesAvailable] = []telemetry.Sample{{Timestamp: now, Value: 5}}
	metrics := &perMetricSource{samples: samples}

	got := CheckNodeAlertStatus(context.Background(), node, &fakeNodeServiceSource{}, metrics, 1, 90, 80, 4<<30, now, nil)
	if got.PatchStatus != NodeAlertFiring {
		t.Errorf("PatchStatus = %v, want firing", got.PatchStatus)
	}
	if got.NodeDiskSpace != NodeAlertOK {
		t.Errorf("NodeDiskSpace = %v, want ok (unaffected by the patch signal)", got.NodeDiskSpace)
	}
}

func TestCheckNodeAlertStatus_DiskSpaceFiring(t *testing.T) {
	now := time.Now()
	node := store.Node{ID: "n1", Name: "web-1"}
	samples := healthyNodeAlertSamples(node, now)
	samples["node:n1"][telemetry.MetricDiskUsedBytes] = []telemetry.Sample{{Timestamp: now, Value: 95}}
	metrics := &perMetricSource{samples: samples}

	got := CheckNodeAlertStatus(context.Background(), node, &fakeNodeServiceSource{}, metrics, 1, 90, 80, 4<<30, now, nil)
	if got.NodeDiskSpace != NodeAlertFiring {
		t.Errorf("NodeDiskSpace = %v, want firing", got.NodeDiskSpace)
	}
	if got.PatchStatus != NodeAlertOK {
		t.Errorf("PatchStatus = %v, want ok (unaffected by the disk signal)", got.PatchStatus)
	}
}

func TestCheckNodeAlertStatus_ResourceUsageFiring(t *testing.T) {
	now := time.Now()
	node := store.Node{ID: "n1", Name: "web-1"}
	samples := healthyNodeAlertSamples(node, now)
	samples["service:svc-a"] = map[string][]telemetry.Sample{
		"cpu_percent": {{Timestamp: now, Value: 95}},
	}
	metrics := &perMetricSource{samples: samples}
	services := &fakeNodeServiceSource{byNode: map[string][]store.DesiredService{
		"n1": {{Name: "svc-a"}},
	}}

	got := CheckNodeAlertStatus(context.Background(), node, services, metrics, 1, 90, 80, 4<<30, now, nil)
	if got.NodeResourceUsage != NodeAlertFiring {
		t.Errorf("NodeResourceUsage = %v, want firing", got.NodeResourceUsage)
	}
	if got.PatchStatus != NodeAlertOK || got.NodeDiskSpace != NodeAlertOK {
		t.Errorf("PatchStatus = %v, NodeDiskSpace = %v, want both ok (unaffected by the resource-usage signal)", got.PatchStatus, got.NodeDiskSpace)
	}
}

func TestCheckNodeAlertStatus_NoSamples_Unknown(t *testing.T) {
	now := time.Now()
	node := store.Node{ID: "n1", Name: "web-1"}
	got := CheckNodeAlertStatus(context.Background(), node, &fakeNodeServiceSource{}, &perMetricSource{}, 1, 90, 80, 4<<30, now, nil)
	if got.PatchStatus != NodeAlertUnknown {
		t.Errorf("PatchStatus = %v, want unknown when there is no recent sample", got.PatchStatus)
	}
	if got.NodeDiskSpace != NodeAlertUnknown {
		t.Errorf("NodeDiskSpace = %v, want unknown when there is no recent sample", got.NodeDiskSpace)
	}
	if got.NodeResourceUsage != NodeAlertUnknown {
		t.Errorf("NodeResourceUsage = %v, want unknown when nothing is placed on this node", got.NodeResourceUsage)
	}
}

func TestCheckNodeAlertStatus_ZeroThresholds_FallBackToDefaults(t *testing.T) {
	now := time.Now()
	node := store.Node{ID: "n1", Name: "web-1"}
	samples := healthyNodeAlertSamples(node, now)
	samples["node:n1"][telemetry.MetricOSSecurityPatchesAvailable] = []telemetry.Sample{{Timestamp: now, Value: 1}}
	metrics := &perMetricSource{samples: samples}

	got := CheckNodeAlertStatus(context.Background(), node, &fakeNodeServiceSource{}, metrics, 0, 0, 0, 0, now, nil)
	if got.PatchStatus != NodeAlertFiring {
		t.Errorf("PatchStatus = %v, want firing at DefaultPatchStatusThreshold (%g) when threshold is 0", got.PatchStatus, float64(DefaultPatchStatusThreshold))
	}
}
