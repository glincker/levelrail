package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

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
	for _, c := range []struct {
		name, env string
		v         int64
	}{
		{"io_read_bps", envMaxIOBps, r.IOReadBps},
		{"io_write_bps", envMaxIOBps, r.IOWriteBps},
		{"net_mbit", envMaxNetMbit, r.NetMbit},
	} {
		if limit := envInt64Cap(c.env); limit > 0 && c.v > limit {
			return resourcesError{fmt.Sprintf("resources.%s %d is above the %d limit (%s)", c.name, c.v, limit, c.env)}
		}
	}
	return nil
}

// Optional operator caps for the throughput fields; unset or invalid means no cap.
const (
	envMaxIOBps   = "APP_RESOURCES_MAX_IO_BPS"
	envMaxNetMbit = "APP_RESOURCES_MAX_NET_MBIT"
)

func envInt64Cap(name string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(name)), 10, 64)
	if err != nil || v < 1 {
		return 0
	}
	return v
}

func writeAppInvalid(w http.ResponseWriter, err error) {
	var re resourcesError
	if errors.As(err, &re) {
		writeCodedError(w, http.StatusBadRequest, invalidResourcesCode, re.msg)
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}
