export type CapacityLevel = 'unknown' | 'ok' | 'warn' | 'critical'

export interface CapacityMetric {
  name: 'disk' | 'inodes' | 'memory' | 'swap'
  level: CapacityLevel
  percent: number
  free: number
  total: number
}

export interface NodeGuardrailSettings {
  disk_warn_free_pct: number | null
  disk_crit_free_pct: number | null
  inode_warn_free_pct: number | null
  inode_crit_free_pct: number | null
  mem_warn_avail_pct: number | null
  mem_crit_avail_pct: number | null
  swap_warn_used_pct: number | null
  swap_crit_used_pct: number | null
  auto_clean: boolean
}

export interface Band {
  warn: number
  critical: number
}

export interface NodeCapacity {
  node_id: string
  local: boolean
  has_reading: boolean
  status: {
    metrics: CapacityMetric[]
    worst: CapacityLevel
    reason?: string
    thresholds: {
      disk_free_pct: Band
      inodes_free_pct: Band
      memory_available_pct: Band
      swap_used_pct: Band
    }
  }
  settings: NodeGuardrailSettings
  cleanup_supported: boolean
  cleanup_unsupported_reason?: string
}

export interface CleanupItem {
  name: string
  detail?: string
  size_bytes: number
  action: string
}

export type CleanupClassName =
  | 'dangling_images'
  | 'build_cache'
  | 'stopped_containers'
  | 'rollback_images'
  | 'orphaned_volumes'

export interface CleanupClass {
  name: CleanupClassName
  deletable: boolean
  bytes: number
  items: CleanupItem[]
}

export interface CleanupResult {
  dry_run: boolean
  preview: { classes: CleanupClass[]; total_bytes: number }
  freed_bytes: number
  error?: string
}

export interface CleanupRun {
  id: string
  started_at: string
  trigger: string
  actor: string
  freed_bytes: number
  classes: CleanupClass[]
  error?: string
}
