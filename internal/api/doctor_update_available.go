package api

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/nodeinsight"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

// doctorCheckUpdateAvailable reads only the release the updates page already
// cached, so the doctor never adds a network probe. It emits nothing when no
// release was ever fetched (checks off or offline) or on a non-stable channel.
func (rt *Router) doctorCheckUpdateAvailable(ctx context.Context) []doctorCheckResource {
	if rt.updatesCache == nil || rt.updateSettings == nil || rt.currentUpdateChannel(ctx) != upgrade.ChannelStable {
		return nil
	}
	release, ok := rt.updatesCache.stale()
	if !ok || release == nil {
		return nil
	}
	const code, name = "update_available", "Control plane update"
	if !upgrade.UpdateAvailable(version.Version, &upgrade.Release{Tag: release.TagName}) {
		return []doctorCheckResource{{Code: code, Name: name, Status: doctorStatusOK, Message: "Running the latest release " + version.Version}}
	}
	return []doctorCheckResource{{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message: fmt.Sprintf("Release %s is available (running %s)", release.TagName, version.Version),
		Fix:     rt.cliName() + " update",
	}}
}

// doctorCheckAgentSkew reuses the node list and the nodeinsight classifier,
// so it reads the database only.
func (rt *Router) doctorCheckAgentSkew(ctx context.Context) []doctorCheckResource {
	if rt.nodes == nil {
		return nil
	}
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		return nil
	}
	in := make([]nodeinsight.NodeVersion, 0, len(nodes))
	for i := range nodes {
		n := nodes[i]
		in = append(in, nodeinsight.NodeVersion{
			ID: n.ID, Name: n.Name, Version: n.AgentVersion, OS: n.AgentOS, Arch: n.AgentArch,
			Online: n.Status == store.NodeStatusOnline, Schedulable: n.Schedulable, Local: rt.isLocalNodeRecord(&n),
		})
	}
	rep := nodeinsight.BuildSkew(version.Version, in)
	const code, name = "agent_version_skew", "Agent versions"
	if rep.Behind == 0 {
		return nil
	}
	return []doctorCheckResource{{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message: fmt.Sprintf("%d agent(s) run an older version than the control plane", rep.Behind),
		Fix:     rt.cliName() + " nodes version-skew",
	}}
}
