//go:build !chaoslive

package main

// The live scenarios pull in the whole control plane (embedded ingress), so the default CLI build leaves them out.
func liveChaosRunner() chaosRunFunc { return nil }

func liveDockerProbe() dockerProbeFunc { return nil }
