package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// newDockerAPIGrants builds the per-service socket source behind docker_api
// grants. It returns nil when no daemon socket can be resolved, so grants are dropped.
func newDockerAPIGrants(ctx context.Context, logger *slog.Logger, settings func() *runtimesettings.Service) *dockerguard.GrantSockets {
	rt := docker.DetectRuntimeSocket(os.LookupEnv)
	upstream, err := dockerguard.UpstreamSocket(rt.Host)
	if err != nil {
		logger.Warn("docker api grants unavailable", slog.String("error", err.Error()))
		return nil
	}
	tun, terr := dockerguard.TunablesFromEnv(os.LookupEnv)
	if terr != nil {
		logger.Warn("dockerguard: tunable invalid, using its default", slog.String("error", terr.Error()))
	}
	hardening, _ := docker.HardeningFromEnv(rt)
	dataDir := dataDirFromEnv()
	policy := dockerguard.PolicyFromHardening(hardening, tun, dataDir)
	policy.ResolveSymlinks = !dockerguard.RunningInContainer()
	policy.GrantSocketDir = dockerguard.GrantSocketDir(dataDir)
	return dockerguard.NewGrantSockets(ctx, dockerguard.GrantSocketsConfig{
		Dir:      policy.GrantSocketDir,
		Upstream: upstream,
		Policy:   policy,
		Tunables: tun,
		// Read per request so toggling the setting applies without a restart.
		Enabled: func() bool {
			rs := settings()
			return rs != nil && rs.Bool(context.Background(), runtimesettings.KeyTemplatesCapDockerAPI)
		},
		Logger: logger,
	})
}
