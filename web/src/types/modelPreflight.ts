// Wire shapes for POST /api/v1/models/preflight and /api/v1/model-cache,
// mirroring internal/models (PreflightResult, CacheReport).

export type PreflightStatus =
  'ok' | 'not_found' | 'gated' | 'rate_limited' | 'unavailable' | 'unsupported'

export type FitVerdict = 'fits' | 'tight' | 'wont_fit' | 'unknown'

export interface PreflightRequest {
  repo: string
  engine?: string
  quant?: string
  file?: string
  node_id?: string
  hf_token?: string
}

export interface PreflightQuant {
  name: string
  bytes: number
  files: string[]
  fit: FitVerdict
  recommended: boolean
}

export interface PreflightSelection {
  label: string
  bytes: number
  fit: FitVerdict
}

export interface PreflightNode {
  node_id: string
  gpu_present: boolean
  vram_total_bytes: number | null
  vram_free_bytes: number | null
  disk_free_bytes: number | null
  disk_total_bytes: number | null
}

export interface PreflightDisk {
  status: 'ok' | 'tight' | 'insufficient' | 'unknown'
  required_bytes: number
  free_bytes: number | null
  message: string
}

export interface PreflightResult {
  repo: string
  status: PreflightStatus
  exists: boolean
  message: string
  next_step?: string
  retry_after_seconds?: number
  gated?: string
  access?: string
  private: boolean
  license?: string
  total_bytes: number
  file_count: number
  files: { name: string; bytes: number }[]
  files_truncated: boolean
  has_gguf: boolean
  has_safetensors: boolean
  compatible_engines: string[]
  engine_hint: string
  quants: PreflightQuant[]
  recommended_quant?: string
  recommendation_note?: string
  selected?: PreflightSelection
  node: PreflightNode
  disk: PreflightDisk
  warnings: string[]
  estimate_note: string
  cached: boolean
}

export interface CacheEntry {
  volume: string
  model?: string
  engine?: string
  model_ref?: string
  size_bytes: number | null
  last_used_at: string | null
  last_used_source: string
  unused_days: number
  unused: boolean
  in_use: boolean
  configured: boolean
  duplicate_of?: string
  prunable: boolean
  keep_reason?: string
}

export interface CacheNode {
  node_id: string
  name: string
  is_local: boolean
  supported: boolean
  message?: string
  entries: CacheEntry[]
  total_bytes: number
  unique_bytes: number
  reclaimable_bytes: number
  disk_free_bytes: number | null
}

export interface CacheReport {
  unused_days: number
  nodes: CacheNode[]
  note: string
}

export interface CachePruneResult {
  dry_run: boolean
  candidates: CacheEntry[]
  removed: string[]
  skipped: { volume: string; reason: string }[]
  reclaimed_bytes: number
}
