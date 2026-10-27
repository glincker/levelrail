package application

import (
	"strconv"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const reasonShmDriftRecreate = "ShmDriftRecreateFailed"

func desiredShm(desired *store.DesiredService) int64 {
	if desired.Resources == nil {
		return 0
	}
	return desired.Resources.ShmSizeBytes
}

// shmSpec stamps the desired shm size as a label (overwriting any user copy) so
// drift is visible from a container listing and a remote agent can apply it.
func shmSpec(desired *store.DesiredService, labels map[string]string) (int64, map[string]string) {
	shm := desiredShm(desired)
	_, had := labels[docker.ShmSizeLabelKey]
	if shm == 0 && !had {
		return 0, labels
	}
	out := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		out[k] = v
	}
	delete(out, docker.ShmSizeLabelKey)
	if shm > 0 {
		out[docker.ShmSizeLabelKey] = strconv.FormatInt(shm, 10)
	}
	return shm, out
}

// shmDrifted reports a container created with a different shm_size than desired.
// Docker cannot resize /dev/shm in place, so the only convergence is a recreate.
func shmDrifted(state *docker.ContainerState, desired *store.DesiredService) bool {
	if state == nil || state.LabelsUnknown {
		return false
	}
	return docker.ShmSizeFromLabels(state.Labels) != desiredShm(desired)
}
