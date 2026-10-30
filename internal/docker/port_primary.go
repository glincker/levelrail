package docker

// PrimaryPort is the TCP binding the proxy and probes dial. Docker's port list
// is unordered and can hold extra published ports, so want (the service's
// routed container port) picks it; want 0 falls back to the first TCP binding.
func PrimaryPort(ports []PortBinding, want int) (PortBinding, bool) {
	for _, p := range ports {
		if (p.Protocol == "" || p.Protocol == "tcp") && (want == 0 || p.ContainerPort == want) {
			return p, true
		}
	}
	return PortBinding{}, false
}
