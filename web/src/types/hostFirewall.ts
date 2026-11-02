// Wire types for /api/v1/firewall/host, matching internal/api/firewall_host.go.

export interface HostFirewallPort {
  port: number
  protocol: 'tcp' | 'udp'
}

export interface HostFirewallPublishedPort extends HostFirewallPort {
  app: string
  service?: string
  bind_address: string
  allowed: boolean
  // false when no firewall filters incoming traffic
  filtered?: boolean
}

export interface HostFirewall {
  installed: boolean
  active: boolean
  backend?: 'ufw' | 'firewalld' | 'nftables' | 'iptables' | 'none'
  managed?: boolean
  detail?: string
  default_incoming?: string
  required: HostFirewallPort[]
  published_ports?: HostFirewallPublishedPort[]
  commands: string[]
}
