package runtimesettings

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Node threshold alert setting keys; per-node overrides live in the node_threshold_overrides table.
const (
	KeyNodeCPUThresholdPercent    = "nodes.alert_cpu_percent"
	KeyNodeMemoryThresholdPercent = "nodes.alert_memory_percent"
	KeyNodeDiskThresholdPercent   = "nodes.alert_disk_percent"
	KeyNodeThresholdSustained     = "nodes.alert_sustained_intervals"

	// NodeThresholdMinPercent and NodeThresholdMaxPercent bound defaults and per-node overrides alike.
	NodeThresholdMinPercent = 1
	NodeThresholdMaxPercent = 100
	nodeSustainedMin        = 1
	nodeSustainedMax        = 60
)

// percentFromEnv accepts a float env value such as 90.5, since the older disk env took floats.
func percentFromEnv(raw string) (string, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return "", fmt.Errorf("%q is not a percent", raw)
	}
	return strconv.Itoa(int(math.Round(f))), nil
}

func init() {
	pct := intRangeCheck(NodeThresholdMinPercent, NodeThresholdMaxPercent)
	Register(Definition{
		Key: KeyNodeCPUThresholdPercent, Category: CategoryObservability, Kind: KindInt, Default: "85",
		Env: "APP_ALERT_NODE_CPU_PERCENT", EnvConvert: percentFromEnv, Apply: ApplyLive, CheckValue: pct,
		Title:       "Node CPU alert percent",
		Description: "Summed CPU percent of the services on a node that a node_cpu rule treats as pressure, from 1 to 100. A node can override it.",
	})
	Register(Definition{
		Key: KeyNodeMemoryThresholdPercent, Category: CategoryObservability, Kind: KindInt, Default: "90",
		Env: "APP_ALERT_NODE_MEMORY_PERCENT", EnvConvert: percentFromEnv, Apply: ApplyLive, CheckValue: pct,
		Title:       "Node memory alert percent",
		Description: "Used host memory percent that a node_memory rule treats as pressure, from 1 to 100. A node can override it.",
	})
	Register(Definition{
		Key: KeyNodeDiskThresholdPercent, Category: CategoryObservability, Kind: KindInt, Default: "90",
		Env: "APP_ALERT_NODE_DISK_SPACE_THRESHOLD_PERCENT", EnvConvert: percentFromEnv, Apply: ApplyLive, CheckValue: pct,
		Title:       "Node disk alert percent",
		Description: "Used disk percent that a node_disk_space rule fires at, from 1 to 100. A node can override it.",
	})
	Register(Definition{
		Key: KeyNodeThresholdSustained, Category: CategoryObservability, Kind: KindInt, Default: "3",
		Env: "APP_ALERT_NODE_SUSTAINED_INTERVALS", Apply: ApplyLive, CheckValue: intRangeCheck(nodeSustainedMin, nodeSustainedMax),
		Title:       "Node pressure sustained intervals",
		Description: "Consecutive alert evaluations a node must stay over its CPU or memory threshold before node_cpu or node_memory fires, from 1 to 60, so one spike does not page.",
	})
}
