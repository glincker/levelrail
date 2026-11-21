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
}

export interface DeployTargetGroup {
  kind: DeployTargetKind
  account_id?: string
  provider?: string
  label: string
  nodes: DeployTargetNode[]
}
