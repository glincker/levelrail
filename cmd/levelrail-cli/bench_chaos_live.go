//go:build chaoslive

package main

import "github.com/GLINCKER/levelrail/internal/chaos/live"

func liveChaosRunner() chaosRunFunc { return live.Run }

func liveDockerProbe() dockerProbeFunc { return live.DockerInfo }
