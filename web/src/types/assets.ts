// Wire types for /api/v1/assets/buckets (internal/api/assets*.go, ADR 044).

export type AssetVisibility = 'private' | 'public'
export type AssetServingMode = 'direct' | 'cdn'

export interface AssetBucket {
  id: string
  name: string
  target_id: string
  preset?: string
  prefix: string
  project_id?: string
  visibility: AssetVisibility
  serving_mode: string
  public_base_url?: string
  cdn_base_url?: string
  referrer_allow: string[]
  refuse_no_referer: boolean
  quota_bytes: number
  created_at: string
  updated_at: string
}

export interface CreateAssetBucketInput {
  name: string
  target_id: string
  prefix?: string
  visibility: AssetVisibility
  serving_mode: AssetServingMode
  public_base_url?: string
  cdn_base_url?: string
  confirm_name?: string
}

export interface UpdateAssetBucketInput {
  visibility?: AssetVisibility
  serving_mode?: AssetServingMode
  public_base_url?: string
  cdn_base_url?: string
  confirm_name?: string
}

export interface AssetItem {
  key: string
  size: number
  last_modified?: string
  content_type?: string
  etag?: string
  uploader?: string
  url?: string
}

export interface AssetList {
  assets: AssetItem[]
  cursor?: string
}

export interface AssetUrls {
  key: string
  visibility: AssetVisibility
  origin_url?: string
  cdn_url?: string
  presigned_url?: string
  presigned_expires_at?: string
  notes: string[]
}

export interface AssetUploadTicket {
  key: string
  method: string
  url: string
  headers: Record<string, string>
  expires_at: string
  max_bytes: number
}

export interface CorsRule {
  allowed_origins: string[]
  allowed_methods: string[]
  allowed_headers: string[]
  expose_headers: string[]
  max_age_seconds: number
}

export type CorsStatus = 'ok' | 'missing' | 'applied' | 'manual' | 'unverified'

export interface CorsCheck {
  status: CorsStatus
  origin: string
  rule: CorsRule
  message?: string
}

export interface ProbeStep {
  name: string
  ok: boolean
  error?: string
}

export interface ProbeWarning {
  code: string
  message: string
  fix?: string
}

export interface AssetProbe {
  ok: boolean
  reason?: string
  message?: string
  fix?: string
  steps: ProbeStep[]
  cors?: CorsCheck
  exposure?: { url: string; public: boolean; status?: number; error?: string }
  warnings: ProbeWarning[]
}

export interface CostLine {
  preset: string
  storage_usd: number
  egress_usd: number
  total_usd: number
  free_egress_gb: number
}

export interface CostEstimate {
  estimated: boolean
  price_table_date: string
  stored_bytes: number
  egress_gb: number
  current: CostLine
  compare: CostLine[]
  override_envs: string[]
  notes: string[]
}

export interface AssetUsage {
  bytes: number
  objects: number
  at_least: boolean
  quota_bytes: number
  serving_mode: string
  traffic_measured: boolean
  cost: CostEstimate
}
