package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/store"
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

func writeAppInvalid(w http.ResponseWriter, err error) {
	var re resourcesError
	if errors.As(err, &re) {
		writeCodedError(w, http.StatusBadRequest, invalidResourcesCode, re.msg)
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}
