// Wire shapes for /api/v1/models and /api/v1/gpus, mirroring
// internal/api/models.go (modelResource, gpuNodeResource).

export type ModelEngine = 'ollama' | 'vllm' | 'llamacpp'

export interface ModelStatus {
  ready: boolean
  reason: string
  message?: string
}

export interface ModelResource {
  name: string
  engine: ModelEngine
  model: string
  node_id: string
  gpu_count: number
  gpu_device_ids?: string[]
  context_length?: number
  quantization?: string
  domain?: string
  endpoint_url?: string
  api_key_prefix: string
  hf_token_set: boolean
  status: ModelStatus
  created_at: string
  updated_at: string
}

export interface CreateModelRequest {
  name: string
  engine: ModelEngine
  model: string
  node_id?: string
  gpu_count?: number
  gpu_device_ids?: string[]
  context_length?: number
  quantization?: string
  domain?: string
  hf_token?: string
}

export interface CreateModelResponse extends ModelResource {
  api_key: string
}

export interface GpuDevice {
  index: number
  uuid: string
  name: string
  vram_total_mib: number
  vram_used_mib: number
  utilization_percent: number
}

export interface GpuNode {
  node_id: string
  name: string
  is_local: boolean
  present: boolean
  driver_version?: string
  runtime_installed: boolean
  gpu_count: number
  total_vram_mib: number
  used_vram_mib: number
  model_count: number
  reserved_gpus: number
  free_gpus: number
  reservations: string[]
  schedulable: boolean
  hint?: string
  devices: GpuDevice[]
  updated_at: string
}

// Wire shapes for /api/v1/models/{name}/keys and /usage, mirroring
// internal/api/models_keys.go and internal/models/usage.go.

export type ModelKeyStatus = 'active' | 'rotating' | 'expired' | 'revoked'

export interface ModelKey {
  id: string
  name: string
  key_prefix: string
  status: ModelKeyStatus
  rpm: number
  tpm: number
  max_parallel: number
  allow_paths: string[]
  allow_models: string[]
  replaced_by?: string
  in_flight: number
  created_at: string
  expires_at?: string
  revoked_at?: string
  last_used_at?: string
}

export interface CreatedModelKey extends ModelKey {
  api_key: string
}

export interface CreateModelKeyRequest {
  name: string
  expires_at?: string
  rpm?: number
  tpm?: number
  max_parallel?: number
  allow_paths?: string[]
  allow_models?: string[]
}

export interface ModelUsageTotals {
  requests: number
  status_2xx: number
  status_4xx: number
  status_5xx: number
  rate_limited: number
  usage_requests: number
  input_tokens: number
  output_tokens: number
  bytes_out: number
  avg_duration_ms: number
  avg_ttft_ms: number
}

export interface ModelUsagePoint extends ModelUsageTotals {
  hour: string
}

export interface ModelKeyUsage extends ModelUsageTotals {
  key_id: string
  name: string
  key_prefix: string
  status: string
  in_flight: number
}

export interface ModelUsageReport {
  model: string
  from: string
  to: string
  totals: ModelUsageTotals
  series: ModelUsagePoint[]
  keys: ModelKeyUsage[]
  in_flight: number
  note: string
}
