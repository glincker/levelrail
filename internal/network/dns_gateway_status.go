package network

// GatewayStatus is the full doctor view: listeners, host firewall and container usage.
type GatewayStatus struct {
	GatewayReport
	FirewallBackend string          `json:"firewall_backend"`
	FirewallPresent map[string]bool `json:"firewall_present,omitempty"`
	FirewallError   string          `json:"firewall_error,omitempty"`
	ContainersUsing int             `json:"containers_using"`
	ContainersTotal int             `json:"containers_total"`
}
