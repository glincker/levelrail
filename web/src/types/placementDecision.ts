export type PlacementDimensionName = 'cpu' | 'memory' | 'disk'

// allocatable and free are null when the node never reported them: unknown,
// not zero and not unlimited. cpu is in millicores, memory and disk in bytes.
export interface PlacementDimension {
  name: PlacementDimensionName
  allocatable: number | null
  requested: number
  need: number
  free: number | null
  shortfall: number
}

export interface PlacementEvaluation {
  node_id: string
  name: string
  eligible: boolean
  reason?: string
  capacity_blocked?: boolean
  unverified?: boolean
  headroom_permille?: number
  load: number
  dimensions: PlacementDimension[]
}

export interface PlacementPolicy {
  cpu_overcommit_percent: number
  memory_overcommit_percent: number
  min_free_disk_percent: number
  reject_unknown: boolean
}

export interface PlacementDecision {
  node_id: string
  why: string
  policy: PlacementPolicy
  considered: PlacementEvaluation[] | null
  warnings?: string[]
}

export interface RecordedPlacement {
  trigger: 'create' | 'move' | 'drain'
  decided_at: string
  decision: PlacementDecision
}

export interface PlacementDecisionView {
  kind: 'app' | 'database'
  name: string
  node_id: string
  recorded: RecordedPlacement | null
  preview: PlacementDecision
}

export interface NodeAllocation {
  node_id: string
  name: string
  is_local: boolean
  schedulable: boolean
  online: boolean
  workloads: number
  dimensions: PlacementDimension[]
}

export interface NodeAllocations {
  policy: PlacementPolicy
  nodes: NodeAllocation[]
}
