export type ExposureClass =
  'loopback' | 'private' | 'restricted' | 'exposed' | 'unknown'
export type ExposureSeverity = 'high' | 'medium' | 'low' | 'info'
export type ExposureOutsideStatus = 'not_run' | 'answers' | 'could_not_confirm'

export interface ExposureOwner {
  kind: 'database' | 'app' | 'unmanaged'
  name?: string
  intentional?: boolean
}

export interface ExposureFinding {
  container: string
  image: string
  owner: ExposureOwner
  image_kind: string
  protocol: string
  host_port: number
  container_port: number
  binds: string[]
  class: ExposureClass
  severity: ExposureSeverity
  allowed_sources?: string[]
  rules?: string[]
  managed: boolean
  explanation: string
  recommendation?: string
  can_restrict: boolean
  cannot_restrict_reason?: string
  restriction?: { allow: string[]; created_at?: string }
  outside_check: { status: ExposureOutsideStatus; detail?: string }
}

export interface ExposureNode {
  node_id: string
  node_name: string
  local: boolean
  status: string
  error?: string
  rules_readable: boolean
  rules_note?: string
  public_address?: string
  findings: ExposureFinding[]
}

export interface ExposureReport {
  generated_at: string
  nodes: ExposureNode[]
  exposed: number
  high: number
}

export interface ExposurePlan {
  commands: string[]
  drops: string
  tag: string
  persistence: string
  port: number
  protocol: string
  allow: string[]
  warnings: string[]
  applied: boolean
}

export interface ExposureRestrictInput {
  port: number
  protocol: string
  allow: string[]
  local_containers: boolean
}
