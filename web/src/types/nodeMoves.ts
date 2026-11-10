import type { DomainRisk } from './nodeOnboarding'
export type NodeMoveKind = 'app' | 'database'

export type NodeMovePhase =
  | 'preflight'
  | 'stop_source'
  | 'copy_volumes'
  | 'place'
  | 'verify'
  | 'stop_old'
  | 'finalize'
  | 'rollback'
  | 'done'

export type NodeMoveStatus = 'running' | 'succeeded' | 'failed' | 'rolled_back'

export type NodeMoveOutcome = 'ok' | 'retry' | 'failed' | 'waiting'

export interface NodeMoveStep {
  phase: NodeMovePhase
  outcome: NodeMoveOutcome
  message?: string
  at: string
}

export interface NodeMove {
  id: string
  kind: NodeMoveKind
  resource: string
  from_node_id: string
  to_node_id: string
  stateful: boolean
  volumes: string[]
  phase: NodeMovePhase
  status: NodeMoveStatus
  error?: string
  drain_id?: string
  steps: NodeMoveStep[]
  started_at: string
  updated_at: string
  finished_at?: string
}

export interface NodeDrainBlocked {
  kind: string
  name: string
  reason: string
}

export type NodeDrainStatus = 'running' | 'succeeded' | 'partial' | 'failed'

export interface NodeDrain {
  id: string
  node_id: string
  target_node_id?: string
  status: NodeDrainStatus
  error?: string
  blocked: NodeDrainBlocked[]
  moves: NodeMove[]
  started_at: string
  updated_at: string
  finished_at?: string
}

export type NodeLabels = Record<string, string>

export interface PlacementRejection {
  node_id: string
  reason: string
}

export type TaintEffect = 'NoSchedule' | 'PreferNoSchedule' | 'NoExecute'

export type TolerationOperator = 'Equal' | 'Exists'

export interface NodeTaint {
  key: string
  value?: string
  effect: TaintEffect
}

export interface PlacementToleration {
  key?: string
  operator?: TolerationOperator
  value?: string
  effect?: TaintEffect
}

export interface AffinityLabelTerm {
  key: string
  op: string
  values?: string[]
}

export interface AffinityWorkloadTerm {
  match: AffinityLabelTerm[]
  topology?: string
  weight?: number
}

export interface PlacementAffinity {
  node_required?: AffinityLabelTerm[]
  node_preferred?: { weight: number; term: AffinityLabelTerm }[]
  together?: AffinityWorkloadTerm[]
  apart?: AffinityWorkloadTerm[]
}

export interface Placement {
  kind: NodeMoveKind
  name: string
  node_id: string
  node_selector: NodeLabels
  tolerations?: PlacementToleration[] | null
  affinity?: PlacementAffinity | null
  eligible_node_ids: string[]
  rejected: PlacementRejection[]
  domain_warnings?: DomainRisk[]
  warnings?: string[]
}

export type MeshPairStatus = 'ok' | 'degraded' | 'down' | 'unknown'

export type MeshLinkState =
  'healthy' | 'stale' | 'never' | 'no_peer' | 'unknown'

export interface MeshLinkView {
  state: MeshLinkState
  last_handshake?: string
  endpoint?: string
}

export interface MeshConnectivityPair {
  a: string
  b: string
  status: MeshPairStatus
  a_to_b: MeshLinkView
  b_to_a: MeshLinkView
  hint?: string
}

export interface MeshConnectivityNode {
  id: string
  name: string
  mesh_address: string
  view_error?: string
}

export interface MeshConnectivity {
  nodes: MeshConnectivityNode[]
  pairs: MeshConnectivityPair[]
  checked_at: string
}

export interface NodeJoinTokenRequest {
  control_plane_addr?: string
  node_name?: string
  mesh?: boolean
  provider?: string
  region?: string
  zone?: string
}
