export interface BucketAppRef {
  app_name: string
  env_prefix: string
}

export interface StorageBucket {
  id: string
  name: string
  target_id: string
  prefix: string
  public: boolean
  public_url?: string
  quota_bytes: number
  app_restarted?: boolean
  lifecycle_days: number
  last_swept_at?: string
  created_at: string
  apps: BucketAppRef[]
  env_keys?: string[]
}

export interface CreateStorageBucketRequest {
  name: string
  target_id: string
  app_name?: string
  env_prefix?: string
  public?: boolean
  quota_bytes?: number
  lifecycle_days?: number
}

export interface UpdateStorageBucketRequest {
  public?: boolean
  quota_bytes?: number
  lifecycle_days?: number
}

export interface BucketObject {
  key: string
  size: number
  last_modified: string
}

export interface BucketObjectsPage {
  objects: BucketObject[]
  next?: string
}

export interface BucketUsage {
  bytes: number
  objects: number
  quota_bytes: number
}

export interface BucketPresign {
  url: string
  method: string
  expires_at: string
}

export interface BucketSweepResult {
  deleted: number
  bytes: number
}
