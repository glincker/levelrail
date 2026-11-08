package application

import (
	"sort"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// applyRuntimeOptions copies the validated runtime block onto the container
// spec. shm_size and read_only travel through Resources, which has its own drift handling.
func applyRuntimeOptions(s *docker.ContainerSpec, r *spec.RuntimeOptions) {
	if r == nil {
		return
	}
	s.Tmpfs, s.Init, s.CapDrop = r.Tmpfs, r.Init != nil && *r.Init, r.CapDrop
	s.DNS = append([]string(nil), r.DNS...)
	s.ExtraHosts = append([]string(nil), r.ExtraHosts...)
	s.PidsLimit = int64(r.PidsLimit)
	s.StopTimeoutSeconds = r.StopTimeout
	if len(r.Sysctls) > 0 {
		s.Sysctls = r.Sysctls
	}
	names := make([]string, 0, len(r.Ulimits))
	for n := range r.Ulimits {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s.Ulimits = append(s.Ulimits, docker.Ulimit{Name: n, Soft: r.Ulimits[n].Soft, Hard: r.Ulimits[n].Hard})
	}
}
