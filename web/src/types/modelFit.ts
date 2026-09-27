// Wire shapes for POST /api/v1/models/fit and GET /api/v1/models/{name}/fit,
// mirroring internal/models/fit_service.go.

export type FitVerdict = 'fits' | 'tight' | 'wont_fit' | 'unknown'

export interface NodeFit {
  node_id: string
  name: string
  is_local: boolean
  eligible: boolean
  current: boolean
  gpus: number
  total_bytes: number
  reserved_bytes: number
  verdict: FitVerdict
  weights_source: 'exact' | 'estimated' | 'unknown'
  weights_bytes: number
  kv_bytes: number
  overhead_bytes: number
  need_bytes: number
  free_bytes: number
  context_tokens: number
  context_assumed: boolean
  arithmetic: string
  reason?: string
  suggestions: string[]
}

export interface FitReport {
  model?: string
  nodes: NodeFit[]
  note: string
}

export interface FitRequest {
  engine: string
  model: string
  quantization?: string
  context_length?: number
  gpu_count?: number
  gpu_device_ids?: string[]
  weights_bytes?: number
  node_id?: string
}
