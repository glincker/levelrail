package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/glincker/levelrail/internal/compose"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

const invalidResourcesCode = "invalid_resources"

// resourcesError is a rejected resources block, answered with its own code.
type resourcesError struct{ msg string }

func (e resourcesError) Error() string { return e.msg }

// validateServiceResources mirrors the compose limits: a negative value would
// lower quota usage, and an uncapped shm is tmpfs no mem_limit bounds.
func validateServiceResources(r *store.ServiceResources) error {
	if r == nil {
		return nil
	}
	for _, f := range []struct {
		name string
		v    int64
	}{
		{"memory_bytes", r.MemoryBytes},
		{"nano_cpus", r.NanoCPUs},
		{"swap_memory_bytes", r.SwapMemoryBytes},
		{"shm_size_bytes", r.ShmSizeBytes},
		{"io_read_bps", r.IOReadBps},
		{"io_write_bps", r.IOWriteBps},
		{"net_mbit", r.NetMbit},
	} {
		if f.v < 0 {
			return resourcesError{fmt.Sprintf("resources.%s must not be negative", f.name)}
		}
	}
	if r.GPU != nil && r.GPU.Count < -1 {
		return resourcesError{"resources.gpu.count must be -1 (every GPU) or more"}
	}
	if limit := compose.MaxShmBytes(); r.ShmSizeBytes > limit {
		return resourcesError{fmt.Sprintf("resources.shm_size_bytes %d is above the %d byte limit (APP_COMPOSE_MAX_SHM_BYTES)", r.ShmSizeBytes, limit)}
	}
	return nil
}

// checkThroughputCaps rejects io and net values above the live operator ceilings; 0 means no ceiling.
func (rt *Router) checkThroughputCaps(ctx context.Context, r *store.ServiceResources) error {
	if r == nil || rt.runtimeSettings == nil {
		return nil
	}
	for _, c := range []struct {
		name, key string
		v         int64
	}{
		{"io_read_bps", runtimesettings.KeyAppsMaxIOBps, r.IOReadBps},
		{"io_write_bps", runtimesettings.KeyAppsMaxIOBps, r.IOWriteBps},
		{"net_mbit", runtimesettings.KeyAppsMaxNetMbit, r.NetMbit},
	} {
		if limit := int64(rt.runtimeSettings.Int(ctx, c.key)); limit > 0 && c.v > limit {
			return resourcesError{fmt.Sprintf("resources.%s %d is above the %d limit (setting %s)", c.name, c.v, limit, c.key)}
		}
	}
	return nil
}

func writeAppInvalid(w http.ResponseWriter, err error) {
	var re resourcesError
	if errors.As(err, &re) {
		writeCodedError(w, http.StatusBadRequest, invalidResourcesCode, re.msg)
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}
