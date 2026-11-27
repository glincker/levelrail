package application

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/store"
)

// WithHostGrants wires the live templates.cap_host_network and templates.cap_devices
// settings. Read on every pass; a nil reader fails closed, so no host grant is applied.
func WithHostGrants(hostNetwork, devices func(context.Context) bool) Option {
	return func(ctrl *Controller) { ctrl.hostNetworkGrant, ctrl.devicesGrant = hostNetwork, devices }
}

func (c *Controller) hostNetworkOn(ctx context.Context) bool {
	return c.hostNetworkGrant != nil && c.hostNetworkGrant(ctx)
}

func (c *Controller) devicesOn(ctx context.Context) bool {
	return c.devicesGrant != nil && c.devicesGrant(ctx)
}

// withoutOffHostGrants drops host network and device tokens whose setting is off now,
// so switching a setting off takes the grant away at the next reconcile.
func (c *Controller) withoutOffHostGrants(ctx context.Context, caps []string) []string {
	hostOn, devOn := c.hostNetworkOn(ctx), c.devicesOn(ctx)
	out := caps[:0:0]
	for _, g := range caps {
		if (dockerguard.IsHostNetworkGrant(g) && !hostOn) || (dockerguard.IsDeviceGrant(g) && !devOn) {
			slog.Warn("reconcile: host grant not applied", slog.String("service", c.serviceName))
			continue
		}
		out = append(out, g)
	}
	return out
}

// applyHostGrants sets host networking and device mappings for a service that holds the grants.
// A grant that cannot be applied returns an error, so the service never starts half granted.
func (c *Controller) applyHostGrants(ctx context.Context, desired *store.DesiredService, spec *docker.ContainerSpec) error {
	if dockerguard.HasHostNetworkGrant(desired.GrantedCaps) {
		if len(plainCaps(desired.GrantedCaps)) > 0 || len(spec.CapAdd) > 0 {
			return fmt.Errorf("host network grant for %s cannot be combined with added capabilities", desired.Name)
		}
		if !c.hostNetworkOn(ctx) {
			return fmt.Errorf("host network grant for %s needs templates.cap_host_network", desired.Name)
		}
		if desired.Egress != nil {
			return fmt.Errorf("host network grant for %s cannot be combined with an egress policy", desired.Name)
		}
		spec.NetworkMode = "host"
		spec.HostNetworkGranted = true
		// Docker ignores published ports on the host network; the app binds its port directly.
		spec.Ports = nil
	}
	devices := dockerguard.DevicesFromCaps(desired.GrantedCaps)
	if len(devices) == 0 {
		return nil
	}
	if !c.devicesOn(ctx) {
		return fmt.Errorf("devices grant for %s needs templates.cap_devices", desired.Name)
	}
	spec.Devices = append(spec.Devices, devices...)
	spec.GrantedDevices = append(spec.GrantedDevices, devices...)
	return nil
}
