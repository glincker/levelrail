package models

import (
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Compute kind and engine name of a model served by an external endpoint.
const (
	ComputeExternal = store.ComputeExternal
	EngineExternal  = "external"
)

// Environment variables that tune external endpoint probing.
const (
	envEndpointSlow           = "APP_MODEL_ENDPOINT_SLOW"
	envEndpointHealthInterval = "APP_MODEL_ENDPOINT_HEALTH_INTERVAL"
)

// EndpointSlowThreshold is the list-models latency above which an endpoint is "slow".
func EndpointSlowThreshold() time.Duration {
	return envDuration(envEndpointSlow, 1500*time.Millisecond)
}

// EndpointHealthInterval is the time between background health probes.
func EndpointHealthInterval() time.Duration {
	if os.Getenv(envEndpointHealthInterval) == "0" {
		return 0
	}
	return envDuration(envEndpointHealthInterval, 20*time.Second)
}
