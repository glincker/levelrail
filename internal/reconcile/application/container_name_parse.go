package application

import "strings"

// ParseContainerName reports the service an app container name belongs to,
// accepting every shape the controller produces: ContainerName, replica
// suffixes and egress sidecars. ok is false for any other name.
func ParseContainerName(name string) (service string, ok bool) {
	base := strings.TrimSuffix(name, egressSidecarSuffix)
	for i := strings.Index(base, "-"); i >= 0; {
		if ownsContainer(base[:i], base) {
			return base[:i], true
		}
		next := strings.Index(base[i+1:], "-")
		if next < 0 {
			break
		}
		i += 1 + next
	}
	return "", false
}
