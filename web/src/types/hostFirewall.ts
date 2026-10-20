// Wire types for /api/v1/firewall/host, matching internal/api/firewall_host.go.

export interface HostFirewallPort {
  port: number
  protocol: 'tcp' | 'udp'
}

export interface HostFirewall {
  installed: boolean
  active: boolean
  default_incoming?: string
  required: HostFirewallPort[]
  commands: string[]
}
