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
