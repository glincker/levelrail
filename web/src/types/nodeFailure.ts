// Wire types for the node failure policy (internal/api/node_failure.go).

export type NodeFailureMode = 'off' | 'notify' | 'auto'

export interface NodeFailureAction {
  kind: 'app' | 'database'
  name: string
  class: 'reschedule' | 'needs_decision'
  status: 'pending' | 'moving' | 'moved' | 'fenced' | 'waiting' | 'failed'
  target?: string
  reason?: string
}

export interface NodeFailureStatus {
  node_id: string
  mode: NodeFailureMode
  grace_seconds: number
  // '' healthy, 'grace' silent inside the grace period, then the live phases.
  phase: '' | 'grace' | 'unreachable' | 'recovering' | 'lost'
  silent_seconds: number
  grace_remaining_seconds: number
  reason?: string
  unreachable_at?: string
  consequences: NodeFailureAction[]
  policy: { node_id: string; mode: string; grace_seconds: number }
}

export interface NodeFailureSummary {
  phase: string
  reason?: string
  needs_decision: number
  moved: number
  unreachable_at?: string
}
