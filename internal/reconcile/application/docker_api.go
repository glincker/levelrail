package application

import (
	"fmt"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// containerDockerSocket is the only path a granted service sees its scoped socket at.
const containerDockerSocket = "/var/run/docker.sock"

// DockerAPISockets hands out a per-service filtered Docker socket. Enabled is
// the live templates.cap_docker_api setting.
type DockerAPISockets interface {
	Enabled() bool
	Ensure(service string, spec dockerguard.ProfileSpec) (hostSocket string, err error)
}

// WithDockerAPI wires the per-grant socket source. Without it a docker_api
// grant is dropped, so the service never gets any Docker access.
func WithDockerAPI(s DockerAPISockets) Option {
	return func(ctrl *Controller) { ctrl.dockerAPI = s }
}

func (c *Controller) dockerAPIOn() bool { return c.dockerAPI != nil && c.dockerAPI.Enabled() }

// applyDockerAPI mounts the service's own scoped socket, scoped to its app.
// Any failure returns an error so the service never starts half granted.
func (c *Controller) applyDockerAPI(desired *store.DesiredService, spec *docker.ContainerSpec) error {
	profile, ok := dockerguard.ProfileFromCaps(desired.GrantedCaps)
	if !ok {
		return nil
	}
	if !c.dockerAPIOn() {
		return fmt.Errorf("docker api grant for %s needs templates.cap_docker_api", desired.Name)
	}
	local := ""
	if c.meshDB != nil {
		local = c.meshDB.LocalNodeID
	}
	if !SameNode(desired.NodeID, "", local) {
		return fmt.Errorf("docker api grant for %s is only supported on the control plane node", desired.Name)
	}
	sock, err := c.dockerAPI.Ensure(desired.Name, dockerguard.ProfileSpec{Profile: profile, Project: desired.AppID, LabelKey: appspec.AppLabelKey})
	if err != nil {
		return fmt.Errorf("docker api socket: %w", err)
	}
	spec.BindMounts = append(spec.BindMounts, docker.BindMount{HostPath: sock, ContainerPath: containerDockerSocket})
	return nil
}

// withoutDockerAPIGrant drops docker_api tokens when the setting is off or no socket source is wired.
func (c *Controller) withoutDockerAPIGrant(kept []string) []string {
	if c.dockerAPIOn() {
		return kept
	}
	out := kept[:0:0]
	for _, g := range kept {
		if !dockerguard.IsGrantCap(g) {
			out = append(out, g)
		}
	}
	if len(out) != len(kept) {
		slog.Warn("reconcile: docker api grant not applied", slog.String("service", c.serviceName))
	}
	return out
}

// plainCaps removes docker_api tokens so only real Linux capabilities reach cap_add.
func plainCaps(caps []string) []string {
	var out []string
	for _, g := range caps {
		if !dockerguard.IsGrantCap(g) {
			out = append(out, g)
		}
	}
	return out
}
