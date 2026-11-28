package main

import (
	"context"
	"fmt"
	"io"
	"time"

	dockerclient "github.com/moby/moby/client"

	"github.com/glincker/levelrail/internal/agent"
	"github.com/glincker/levelrail/internal/build"
	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/runtimesettings"
)

// warmFallbackInterval guards the loop from a zero wait if the setting is unreadable.
const warmFallbackInterval = 6 * time.Hour

// buildWarmOption wires base image warming: the list is read live, an empty list pulls nothing.
func buildWarmOption(settings *runtimesettings.Service, registry *agent.Registry, local *dockerclient.Client) build.RouterOption {
	images := func(ctx context.Context) []string {
		return settings.List(ctx, runtimesettings.KeyBuildWarmBaseImages)
	}
	return build.WithRouterWarm(images, warmPull(registry, local))
}

// warmPull pulls on the control plane for an empty node ID and through the agent for a remote node.
func warmPull(registry *agent.Registry, local *dockerclient.Client) func(ctx context.Context, nodeID, ref string) error {
	return func(ctx context.Context, nodeID, ref string) error {
		if nodeID == "" {
			rc, err := local.ImagePull(ctx, ref, dockerclient.ImagePullOptions{})
			if err != nil {
				return fmt.Errorf("pull %s: %w", ref, err)
			}
			defer func() { _ = rc.Close() }()
			_, err = io.Copy(io.Discard, rc)
			return err
		}
		t, err := registry.Get(nodeID)
		if err != nil {
			return err
		}
		p, ok := t.(docker.ImagePuller)
		if !ok {
			return agent.ErrImagePullUnsupported
		}
		return p.PullImage(ctx, ref, nil)
	}
}

// runBaseWarm starts the warm loop; interval and timeout are read live each round.
func runBaseWarm(ctx context.Context, router *build.Router, settings *runtimesettings.Service) {
	interval := func(c context.Context) time.Duration {
		if d := settings.Duration(c, runtimesettings.KeyBuildWarmBaseInterval); d > 0 {
			return d
		}
		return warmFallbackInterval
	}
	timeout := func(c context.Context) time.Duration {
		return settings.Duration(c, runtimesettings.KeyBuildWarmBaseTimeout)
	}
	go router.RunBaseWarm(ctx, interval, timeout)
}
