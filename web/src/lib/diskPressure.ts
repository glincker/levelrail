import type { SystemStatus } from '../queries/systemStatus'

export type DiskPressureLevel = 'ok' | 'warning' | 'critical'

export interface DiskPressure {
  level: DiskPressureLevel
  freePercent: number
  freeBytes: number
  reclaimableBytes: number
}

function envPercent(raw: string | undefined, fallback: number): number {
  const n = Number(raw)
  return raw && Number.isFinite(n) && n > 0 && n < 100 ? n : fallback
}

const WARN_FREE_PERCENT = envPercent(
  import.meta.env.VITE_DISK_WARN_FREE_PERCENT as string | undefined,
  15,
)
const CRITICAL_FREE_PERCENT = envPercent(
  import.meta.env.VITE_DISK_CRITICAL_FREE_PERCENT as string | undefined,
  5,
)

export function assessDiskPressure(
  status: Pick<
    SystemStatus,
    'data_dir_total_bytes' | 'data_dir_free_bytes' | 'docker_disk_usage'
  >,
  warnAt = WARN_FREE_PERCENT,
  criticalAt = CRITICAL_FREE_PERCENT,
): DiskPressure | undefined {
  const total = status.data_dir_total_bytes
  const free = status.data_dir_free_bytes
  if (!total || free === undefined) {
    return undefined
  }
  const freePercent = (free / total) * 100
  const usage = status.docker_disk_usage
  const reclaimableBytes = usage
    ? usage.images_reclaimable_bytes +
      usage.containers_reclaimable_bytes +
      usage.volumes_reclaimable_bytes +
      usage.build_cache_reclaimable_bytes
    : 0
  let level: DiskPressureLevel = 'ok'
  if (freePercent < criticalAt) {
    level = 'critical'
  } else if (freePercent < warnAt) {
    level = 'warning'
  }
  return { level, freePercent, freeBytes: free, reclaimableBytes }
}
