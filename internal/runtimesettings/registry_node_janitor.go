package runtimesettings

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Remote node janitor keys.
const (
	KeyJanitorRemoteExecute    = "janitor.remote_execute"
	KeyJanitorRemoteInterval   = "janitor.remote_interval_minutes"
	KeyJanitorNodeBudgetGB     = "janitor.node_images_budget_gb"
	KeyJanitorNodeBudgetByNode = "janitor.node_images_budget_overrides"
)

const (
	janitorBudgetMaxGB   = 100000
	janitorIntervalMin   = 5
	janitorIntervalMax   = 7 * 24 * 60
	janitorOverridesMax  = 10000
	janitorBudgetDefault = "10"
)

// ParseNodeBudgetOverrides reads the node to budget-in-GB map; an empty value is no overrides.
func ParseNodeBudgetOverrides(raw string) (map[string]int, error) {
	out := map[string]int{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("must be a JSON object of node id to budget in GB: %w", err)
	}
	if len(out) > janitorOverridesMax {
		return nil, fmt.Errorf("at most %d nodes", janitorOverridesMax)
	}
	for id, gb := range out {
		if id == "" {
			return nil, errors.New("node ids must not be empty")
		}
		if gb < 1 || gb > janitorBudgetMaxGB {
			return nil, fmt.Errorf("node %q: budget must be from 1 to %d GB", id, janitorBudgetMaxGB)
		}
	}
	return out, nil
}

func init() {
	Register(Definition{
		Key: KeyJanitorRemoteExecute, Category: CategoryRetention, Kind: KindBool, Default: "false", Apply: ApplyLive,
		Title:       "Remote node cleanup: remove for real",
		Description: "Off by default: the scheduled cleanup of remote nodes only previews and records what it would free. On lets it remove unused data on nodes whose auto-clean is enabled, using the same safe classes as the manual cleanup.",
	})
	Register(Definition{
		Key: KeyJanitorRemoteInterval, Category: CategoryRetention, Kind: KindInt, Default: "360", Apply: ApplyLive,
		CheckValue:  intRangeCheck(janitorIntervalMin, janitorIntervalMax),
		Title:       "Remote node cleanup: interval",
		Description: "Minutes between scheduled cleanup passes on each remote node, from 5 to 10080.",
	})
	Register(Definition{
		Key: KeyJanitorNodeBudgetGB, Category: CategoryRetention, Kind: KindInt, Default: janitorBudgetDefault, Apply: ApplyLive,
		CheckValue:  intRangeCheck(1, janitorBudgetMaxGB),
		Title:       "Node disk budget for unused images",
		Description: "GB of unused images a node may hold before the janitor lists the oldest ones for removal, from 1 to 100000. A node can have its own value.",
	})
	Register(Definition{
		Key: KeyJanitorNodeBudgetByNode, Category: CategoryRetention, Kind: KindString, Default: "", Apply: ApplyLive,
		CheckValue: func(canonical string) error {
			_, err := ParseNodeBudgetOverrides(canonical)
			return err
		},
		Title:       "Node disk budget, per node",
		Description: "Budgets set on single nodes, as a JSON object of node id to GB. A node without an entry follows the default budget.",
	})
}
