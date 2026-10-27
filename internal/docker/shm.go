package docker

import "strconv"

// ShmSizeLabelKey records the requested /dev/shm size on the container. It
// rides the agent transport's existing labels, so a remote node applies it
// without a protocol change. Must stay under spec.ReservedLabelPrefix.
const ShmSizeLabelKey = "platform-reserved.shm-size-bytes"

// ShmSizeFromLabels reads ShmSizeLabelKey; absent or invalid is 0 (Docker's default).
func ShmSizeFromLabels(labels map[string]string) int64 {
	n, err := strconv.ParseInt(labels[ShmSizeLabelKey], 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func effectiveShmSize(spec ContainerSpec) int64 {
	if spec.ShmSizeBytes > 0 {
		return spec.ShmSizeBytes
	}
	return ShmSizeFromLabels(spec.Labels)
}
