package docker

// PrimaryPort is the first TCP binding: the proxy and probes dial it, and a
// UDP published port can sort first in Docker's port map.
func PrimaryPort(ports []PortBinding) (PortBinding, bool) {
	for _, p := range ports {
		if p.Protocol == "" || p.Protocol == "tcp" {
			return p, true
		}
	}
	return PortBinding{}, false
}
