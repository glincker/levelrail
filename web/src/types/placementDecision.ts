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

export interface ZoneSummary {
  zone: string
  region?: string
  providers?: string[]
  node_ids: string[]
  online_nodes: number
  workloads: number
}

export interface ZoneNode {
  node_id: string
  name: string
  zone?: string
  region?: string
  provider?: string
  online: boolean
  workloads: number
}

export interface ZonesView {
  zones: ZoneSummary[]
  nodes: ZoneNode[]
  labeled: boolean
}

export interface SimulatedPlacement {
  name: string
  from_node_id?: string
  node_id?: string
  node_name?: string
  zone?: string
  why?: string
  unplaced?: string
}

// Reserved versus used memory per node; capacity and used are 0 when the
// host never reported them.
export interface NodeReservation {
  node_id: string
  name: string
  cpu_milli_reserved: number
  cpu_milli_capacity?: number
  memory_reserved_bytes: number
  memory_capacity_bytes?: number
  memory_used_bytes?: number
  memory_stranded_bytes: number
}

export interface PlacementSimulation {
  nodes?: NodeReservation[]
  placed: SimulatedPlacement[]
  unplaced: number
  zones: ZoneSummary[]
}

export interface FailureSimulation extends PlacementSimulation {
  zone?: string
  node?: string
  lost_node_ids: string[]
  displaced: number
}
