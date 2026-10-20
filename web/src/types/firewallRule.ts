// Wire types for the firewall-rule resource, matching
// internal/api/firewall_rules.go's firewallRuleResource.

export type FirewallRuleAction = 'allow' | 'deny'
export type FirewallRuleProtocol = 'tcp' | 'udp'

export interface FirewallRule {
  id: string
  node_id: string
  port: number
  protocol: FirewallRuleProtocol
  source_cidr?: string
  action: FirewallRuleAction
  label?: string
  created_at: string
}

export interface CreateFirewallRuleRequest {
  port: number
  protocol?: FirewallRuleProtocol
  source_cidr?: string
  action?: FirewallRuleAction
  label?: string
}
