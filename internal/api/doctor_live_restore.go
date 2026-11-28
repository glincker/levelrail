package api

import (
	"context"
	"runtime"

	"github.com/glincker/levelrail/internal/docker"
)

// liveRestorer is implemented by the Docker client behind DockerPinger.
type liveRestorer interface {
	LiveRestorePosture(ctx context.Context) (liveRestore, swarm bool, err error)
}

// doctorCheckLiveRestore warns when a Docker daemon restart would stop every
// container. It only reads daemon info and never rewrites daemon.json.
func (rt *Router) doctorCheckLiveRestore(ctx context.Context) doctorCheckResource {
	const code, name = "docker_live_restore", "Docker live-restore"
	skip := func(msg string) doctorCheckResource {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: msg}
	}
	lr, ok := rt.dockerPinger.(liveRestorer)
	if !ok {
		return skip("daemon settings are not available from this client")
	}
	if runtime.GOOS != "linux" || rt.detectRuntimeInfo(ctx).Kind == docker.RuntimeKindPodman {
		return skip("live-restore applies to the Docker daemon on Linux only")
	}
	on, swarm, err := lr.LiveRestorePosture(ctx)
	if err != nil {
		return skip("daemon settings could not be read")
	}
	if swarm {
		return skip("live-restore does not apply in swarm mode")
	}
	if on {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "live-restore is on, containers keep running through a Docker restart"}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn,
		Message: "live-restore is off: restarting or upgrading Docker stops every container on this host",
		Fix:     `Set "live-restore": true in /etc/docker/daemon.json, then run systemctl reload docker. The reload is enough, no daemon restart is needed. It does not apply in swarm mode. Levelrail does not edit this file.`}
}
