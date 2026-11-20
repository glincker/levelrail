package runtimesettings

import (
	"context"
	"time"
)

const (
	// KeyBuildkitProbeTimeout bounds the check that the engine serves BuildKit itself.
	KeyBuildkitProbeTimeout = "build.buildkit_probe_timeout"
	// KeyBuildkitReadyTimeout bounds the wait for the managed builder to answer.
	KeyBuildkitReadyTimeout = "build.buildkit_ready_timeout"
	// KeyBuildkitInfoTimeout bounds one readiness call to the managed builder.
	KeyBuildkitInfoTimeout = "build.buildkit_info_timeout"
	// KeyBuildkitRetryInterval spaces readiness calls while the builder starts.
	KeyBuildkitRetryInterval = "build.buildkit_retry_interval"
)

const (
	buildkitProbeFloor = time.Second
	buildkitProbeCeil  = 2 * time.Minute
	buildkitReadyFloor = 5 * time.Second
	buildkitReadyCeil  = 30 * time.Minute
	buildkitInfoFloor  = time.Second
	buildkitInfoCeil   = time.Minute
	buildkitRetryFloor = 100 * time.Millisecond
	buildkitRetryCeil  = 30 * time.Second
)

// BuildkitFallbackTimeouts returns the live timings for the managed builder fallback.
func (s *Service) BuildkitFallbackTimeouts(ctx context.Context) (probe, ready, info, retry time.Duration) {
	return s.Duration(ctx, KeyBuildkitProbeTimeout), s.Duration(ctx, KeyBuildkitReadyTimeout),
		s.Duration(ctx, KeyBuildkitInfoTimeout), s.Duration(ctx, KeyBuildkitRetryInterval)
}

func init() {
	Register(Definition{
		Key: KeyBuildkitProbeTimeout, Category: CategoryDeploys, Kind: KindDuration, Default: "10s", Env: "APP_BUILDKIT_PROBE_TIMEOUT", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(buildkitProbeFloor, buildkitProbeCeil),
		Title:       "Builder check timeout",
		Description: "How long to wait, from 1s to 2m, when checking whether the container engine has BuildKit built in. If it does not, builds use a builder container that Levelrail starts. A change applies the next time the build client connects.",
	})
	Register(Definition{
		Key: KeyBuildkitReadyTimeout, Category: CategoryDeploys, Kind: KindDuration, Default: "90s", Env: "APP_BUILDKIT_READY_TIMEOUT", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(buildkitReadyFloor, buildkitReadyCeil),
		Title:       "Builder start timeout",
		Description: "How long to wait, from 5s to 30m, for the builder container to start and answer, including the first image pull. Raise it on slow networks. A change applies the next time the build client connects.",
	})
	Register(Definition{
		Key: KeyBuildkitInfoTimeout, Category: CategoryDeploys, Kind: KindDuration, Default: "5s", Env: "APP_BUILDKIT_INFO_TIMEOUT", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(buildkitInfoFloor, buildkitInfoCeil),
		Title:       "Builder ready check timeout",
		Description: "How long one readiness check of the builder container may take, from 1s to 1m. A change applies the next time the build client connects.",
	})
	Register(Definition{
		Key: KeyBuildkitRetryInterval, Category: CategoryDeploys, Kind: KindDuration, Default: "1s", Env: "APP_BUILDKIT_RETRY_INTERVAL", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(buildkitRetryFloor, buildkitRetryCeil),
		Title:       "Builder ready check interval",
		Description: "Pause between readiness checks while the builder container starts, from 100ms to 30s. A change applies the next time the build client connects.",
	})
}
