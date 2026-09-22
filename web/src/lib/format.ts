// Human-readable formatting for values the backend expresses in raw
// machine units: internal/store.ServiceResources (bytes, nano-CPUs) and
// internal/store.ServiceProbe (nanosecond time.Duration, marshaled as a
// plain number since time.Duration has no custom MarshalJSON). Per that
// package's own doc comment, translating "512Mi"/"0.5 cores" style units
// happens in the deploy pipeline on the way in; translating raw units
// back to something readable on the way out is this display layer's job.

export function formatBytes(bytes?: number | null): string {
  if (!bytes || bytes <= 0) {
    return 'not set'
  }
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let value = bytes
  let unitIndex = 0
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex += 1
  }
  const decimals = unitIndex > 0 && value < 10 ? 1 : 0
  return `${value.toFixed(decimals)} ${units[unitIndex]}`
}

export function formatNanoCpus(nanoCpus?: number | null): string {
  if (!nanoCpus || nanoCpus <= 0) {
    return 'not set'
  }
  const cores = nanoCpus / 1_000_000_000
  return `${cores.toFixed(2)} cores`
}

// formatDate renders an RFC3339 timestamp (as every attempt-history
// record in this app carries: BackupHistoryRecord.started_at,
// RestoreHistoryRecord.started_at, BackupVerificationRecord.started_at,
// and each type's own optional finished_at) in the viewer's local time,
// falling back to a placeholder for a field that hasn't been filled in
// yet (e.g. finished_at on a still-running attempt).
export function formatDate(iso: string | undefined, fallback: string): string {
  return iso ? new Date(iso).toLocaleString() : fallback
}

// formatAge renders an RFC3339 timestamp as a relative "Set N days ago"
// style string for a secret/env var's last-set time
// (SecretKeyState.updatedAt, SharedEnvVar.updatedAt). Falls back to a
// placeholder for an unset/unparseable timestamp, the same "absence is
// not an error" shape formatDate's own fallback param already has.
export function formatAge(
  iso: string | undefined,
  fallback = 'unknown',
): string {
  if (!iso) return fallback
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return fallback

  const diffMs = Date.now() - then
  const diffSeconds = Math.floor(diffMs / 1000)
  if (diffSeconds < 60) return 'just now'

  const diffMinutes = Math.floor(diffSeconds / 60)
  if (diffMinutes < 60) {
    return `${diffMinutes} minute${diffMinutes === 1 ? '' : 's'} ago`
  }

  const diffHours = Math.floor(diffMinutes / 60)
  if (diffHours < 24) {
    return `${diffHours} hour${diffHours === 1 ? '' : 's'} ago`
  }

  const diffDays = Math.floor(diffHours / 24)
  if (diffDays < 30) {
    return `${diffDays} day${diffDays === 1 ? '' : 's'} ago`
  }

  const diffMonths = Math.floor(diffDays / 30)
  if (diffMonths < 12) {
    return `${diffMonths} month${diffMonths === 1 ? '' : 's'} ago`
  }

  const diffYears = Math.floor(diffDays / 365)
  return `${diffYears} year${diffYears === 1 ? '' : 's'} ago`
}

// ageInDays converts an RFC3339 timestamp to a whole number of days
// elapsed since, for a numeric "N days" display rather than formatAge's
// prose. Returns null for an unset/unparseable timestamp.
export function ageInDays(iso: string | undefined): number | null {
  if (!iso) return null
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return null
  return Math.floor((Date.now() - then) / (1000 * 60 * 60 * 24))
}

export function formatDurationNs(nanoseconds?: number | null): string {
  if (!nanoseconds || nanoseconds <= 0) {
    return 'not set'
  }
  const seconds = nanoseconds / 1_000_000_000
  if (seconds < 1) {
    return `${Math.round(nanoseconds / 1_000_000)} ms`
  }
  if (seconds < 60) {
    return `${seconds.toFixed(seconds < 10 ? 1 : 0)} s`
  }
  const minutes = seconds / 60
  return `${minutes.toFixed(1)} min`
}
