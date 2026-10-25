package database

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
)

const (
	// EnvMinSafeAgentVersion is the first agent release that reports every
	// field the database drift checks compare (container labels).
	EnvMinSafeAgentVersion = "APP_AGENT_MIN_SAFE_VERSION"

	defaultMinSafeAgentVersion = "0.8.5"

	reasonAgentOutdated = "AgentOutdated"
	// ConditionTypeAgentCompatible is the extra condition set while a
	// database's node runs an agent too old for safe drift decisions.
	ConditionTypeAgentCompatible = "AgentCompatible"

	reasonDataVolumeMissing = "DataVolumeMissing"
)

// AgentVersionLookup returns the agent build a node reported. remote is false
// for the local node, which has no agent and is never outdated.
type AgentVersionLookup func(ctx context.Context, nodeID string) (version string, remote bool, err error)

// WithAgentVersion enables the outdated-agent gate: drift that would replace a
// container is ignored while the node's agent is older than the minimum.
func WithAgentVersion(fn AgentVersionLookup) Option {
	return func(c *Controller) { c.agentVersion = fn }
}

var agentWarned sync.Map

// versionBelow reports whether version sorts before floor. A pre-release of floor
// counts as below, and an unparseable or empty version counts as below, so an
// unknown agent is never trusted with a destructive decision.
func versionBelow(version, floor string) bool {
	core, pre := splitVersion(version)
	floorCore, _ := splitVersion(floor)
	have, ok := parseCore(core)
	want, wok := parseCore(floorCore)
	if !ok || !wok {
		return true
	}
	for i := range have {
		if have[i] != want[i] {
			return have[i] < want[i]
		}
	}
	return pre != ""
}

func splitVersion(v string) (core, pre string) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	core, pre, _ = strings.Cut(v, "-")
	return core, pre
}

func parseCore(core string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// agentOutdated is true when the database's node runs an agent below the
// minimum. A failed lookup is treated as outdated: no data is risked.
func (c *Controller) agentOutdated(ctx context.Context, nodeID string) bool {
	if c.agentVersion == nil {
		return false
	}
	version, remote, err := c.agentVersion(ctx, nodeID)
	if !remote && err == nil {
		return false
	}
	floor := os.Getenv(EnvMinSafeAgentVersion)
	if floor == "" {
		floor = defaultMinSafeAgentVersion
	}
	if err == nil && !versionBelow(version, floor) {
		return false
	}
	if _, seen := agentWarned.LoadOrStore(nodeID, true); !seen {
		slog.Warn("agent too old to make safe database decisions, upgrade it",
			slog.String("node_id", nodeID), slog.String("agent_version", version), slog.String("minimum", floor))
	}
	return true
}

// withAgentNote adds the AgentOutdated condition next to the normal result.
func withAgentNote(res reconcile.Result, outdated bool) reconcile.Result {
	if !outdated {
		return res
	}
	res.Conditions = append(res.Conditions, reconcile.Condition{
		Type: ConditionTypeAgentCompatible, Status: reconcile.ConditionFalse, Reason: reasonAgentOutdated,
		Message: "the node's agent is too old to make safe database decisions; upgrade it. Only an image change or an explicit stop acts until then.",
	})
	return res
}

// checkNamedMounts refuses a spec that would give the data container an
// anonymous volume instead of the named one.
func checkNamedMounts(spec docker.ContainerSpec, dataVolume string) error {
	if dataVolume == "" {
		return nil
	}
	found := false
	for _, v := range spec.Volumes {
		if v.Name == "" {
			return fmt.Errorf("refusing to create %q: mount at %s has no named volume", spec.Name, v.ContainerPath)
		}
		if v.Name == dataVolume {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("refusing to create %q: data volume %q is not mounted", spec.Name, dataVolume)
	}
	return nil
}

// requireVolume runs before any replace: the spec must mount the existing
// named data volume, and that volume must still exist, otherwise the
// replacement would start on an empty data directory. Runtimes that cannot
// list volumes (remote agents) skip the existence check.
func (c *Controller) requireVolume(ctx context.Context, volName string, spec docker.ContainerSpec) error {
	if len(spec.Volumes) == 0 {
		return nil
	}
	if err := checkNamedMounts(spec, volName); err != nil {
		return err
	}
	adm, ok := c.runtime.(volumeAdmin)
	if !ok {
		return nil
	}
	vols, err := adm.ListNamedVolumes(ctx)
	if err != nil {
		return fmt.Errorf("list volumes before replace: %w", err)
	}
	for _, v := range vols {
		if v.Name == volName {
			return nil
		}
	}
	return &dataVolumeMissingError{name: volName}
}

type dataVolumeMissingError struct{ name string }

func (e *dataVolumeMissingError) Error() string {
	return fmt.Sprintf("data volume %q does not exist although its container does; not replacing onto an empty one, restore the volume or delete the database", e.name)
}
