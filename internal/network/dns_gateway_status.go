package network

// DNSFirewallFinding mirrors the host firewall finding so this package stays a low-level leaf.
type DNSFirewallFinding struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Command string `json:"command"`
}

// GatewayStatus is the full doctor view: listeners, host firewall and container usage.
type GatewayStatus struct {
	GatewayReport
	FirewallBackend string          `json:"firewall_backend"`
	FirewallPresent map[string]bool `json:"firewall_present,omitempty"`
	FirewallError   string          `json:"firewall_error,omitempty"`
	// FirewallFindings are detected problems with a printed fix; we never run the fix.
	FirewallFindings []DNSFirewallFinding `json:"firewall_findings,omitempty"`
	ContainersUsing  int                  `json:"containers_using"`
	ContainersTotal  int                  `json:"containers_total"`
}
