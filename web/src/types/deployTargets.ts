// Wire types for GET /api/v1/deploy-targets (internal/api/deploy_targets.go).
export type DeployTargetKind = 'local' | 'account' | 'unmanaged'

export interface DeployTargetNode {
  id: string
  name: string
  status: string
  health: 'healthy' | 'offline' | 'cordoned' | 'pending'
  schedulable: boolean
  region?: string
  tags?: Record<string, string>
  is_local?: boolean
  monthly_cost_estimate?: number
  cost_currency?: string
  free_capacity?: { cpu_cores?: number; memory_bytes?: number }
}

export interface DeployTargetGroup {
  kind: DeployTargetKind
  account_id?: string
  provider?: string
  label: string
  nodes: DeployTargetNode[]
}
