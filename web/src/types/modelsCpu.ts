// Wire shapes for CPU mode, mirroring internal/models (CatalogView, CPUFit,
// BenchResult) and internal/api/models_cpu.go.

export type ModelCompute = 'gpu' | 'cpu'
export type LocalModelTier = 'light' | 'default' | 'plus'
export type CpuFitVerdict =
  'fits_comfortably' | 'tight' | 'wont_fit' | 'unknown'

export interface ModelDownload {
  done_bytes: number
  total_bytes: number
  percent: number
}

export interface BenchPrompt {
  name: string
  prompt_tokens: number
  completion_tokens: number
  ttft_ms: number
  prompt_tokens_per_sec: number
  gen_tokens_per_sec: number
}

export interface ModelBenchmark {
  model: string
  ran_at: string
  duration_ms: number
  prompts: BenchPrompt[]
  prompt_tokens_per_sec: number
  gen_tokens_per_sec: number
  ttft_ms: number
  peak_memory_bytes: number
  memory_limit_bytes: number
  truncated: boolean
  note: string
}

export interface CpuFit {
  verdict: CpuFitVerdict
  reason: string
  suggestion?: string
  suggested_id?: string
  cpu_milli: number
  memory_mib: number
  context_tokens: number
  weights_bytes: number
  weights_source: string
  kv_bytes: number
  overhead_bytes: number
  need_bytes: number
  cap_bytes: number
  disk_bytes: number
  disk_ok: boolean | null
  tokens_per_sec_low: number
  tokens_per_sec_high: number
  estimate: boolean
  arithmetic: string
  warnings: string[]
  note: string
}

export interface CpuFitReport extends CpuFit {
  node_id: string
  host_known: boolean
  host_cpus: number
  host_memory_bytes: number
  host_memory_available_bytes: number
  disk_free_bytes: number | null
  entry?: LocalModel
  license_curated: boolean
  license_notice?: string
}

export interface LocalModel {
  id: string
  label: string
  vendor: string
  repo: string
  file: string
  params_b: number
  quant: string
  download_bytes: number
  sha256: string
  min_ram_mib: number
  context_length: number
  license: string
  tier: LocalModelTier
  kind: 'chat' | 'embedding'
  tool_calling: boolean
  notes: string
}

export interface LocalModelCard extends LocalModel {
  fit: CpuFit
}

export interface CpuBounds {
  default_cpu_milli: number
  default_memory_mib: number
  max_cpu_milli: number
  max_memory_mib: number
  free_ram_floor_mib: number
}

export interface LocalModelCatalog {
  version: number
  verified_on: string
  note: string
  node_id: string
  host_known: boolean
  host_cpus: number
  host_memory_bytes: number
  bounds: CpuBounds
  models: LocalModelCard[]
}

export interface CpuFitRequest {
  compute: 'cpu'
  catalog_id?: string
  model?: string
  context_length?: number
  cpus?: number
  memory_mib?: number
  weights_bytes?: number
}
