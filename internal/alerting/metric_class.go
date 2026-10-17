package alerting

import (
	"sort"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// MetricClass says whether a threshold metric is a request-path signal or a real resource or runtime one.
type MetricClass string

const (
	// MetricClassRequestPath signals are meaningless while the ingress answers 503 itself.
	MetricClassRequestPath MetricClass = "request_path"
	// MetricClassResource signals stay real during maintenance: the container keeps running.
	MetricClassResource MetricClass = "resource"
)

// metricClasses is the single source of truth. A metric missing here is
// treated as MetricClassResource so a new metric alerts until classified.
var metricClasses = map[string]MetricClass{
	telemetry.MetricHTTPRequests:       MetricClassRequestPath,
	telemetry.MetricHTTPResponses2xx:   MetricClassRequestPath,
	telemetry.MetricHTTPResponses3xx:   MetricClassRequestPath,
	telemetry.MetricHTTPResponses4xx:   MetricClassRequestPath,
	telemetry.MetricHTTPResponses5xx:   MetricClassRequestPath,
	telemetry.MetricHTTPUpstreamErrors: MetricClassRequestPath,
	telemetry.MetricHTTPBytesIn:        MetricClassRequestPath,
	telemetry.MetricHTTPBytesOut:       MetricClassRequestPath,
	MetricRequestErrorRate5xxPct:       MetricClassRequestPath,
	MetricRequestP95Ms:                 MetricClassRequestPath,
	MetricRequestP99Ms:                 MetricClassRequestPath,
	telemetry.MetricLBUpstreamsHealthy: MetricClassRequestPath,
	telemetry.MetricLBActiveRequests:   MetricClassRequestPath,

	"cpu_percent":                         MetricClassResource,
	"memory_usage_bytes":                  MetricClassResource,
	"memory_limit_bytes":                  MetricClassResource,
	"network_rx_bytes":                    MetricClassResource,
	"network_tx_bytes":                    MetricClassResource,
	"disk_read_bytes":                     MetricClassResource,
	"disk_write_bytes":                    MetricClassResource,
	telemetry.MetricDiskUsedBytes:         MetricClassResource,
	telemetry.MetricDiskTotalBytes:        MetricClassResource,
	telemetry.MetricMemoryAvailableBytes:  MetricClassResource,
	telemetry.MetricMemoryTotalBytes:      MetricClassResource,
	telemetry.MetricSwapUsedBytes:         MetricClassResource,
	telemetry.MetricContainerRestartCount: MetricClassResource,
	telemetry.MetricDeployCount:           MetricClassResource,
	telemetry.MetricBuildDuration:         MetricClassResource,
	telemetry.MetricLBUpstreamsTotal:      MetricClassResource,
}

func init() {
	for i := 0; i < telemetry.LatencyBucketCount; i++ {
		metricClasses[telemetry.LatencyBucketMetric(i)] = MetricClassRequestPath
	}
}

// ClassifyMetric returns the class of metric; unknown metrics are resource signals.
func ClassifyMetric(metric string) MetricClass {
	if c, ok := metricClasses[metric]; ok {
		return c
	}
	return MetricClassResource
}

// RequestPathMetrics lists every metric classed as request-path, sorted.
func RequestPathMetrics() []string {
	out := make([]string, 0, len(metricClasses))
	for m, c := range metricClasses {
		if c == MetricClassRequestPath {
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}
