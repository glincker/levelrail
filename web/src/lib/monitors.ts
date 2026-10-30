import type { Monitor, MonitorResults } from '../types/monitor'

export type MonitorState =
  'up' | 'suspect' | 'down' | 'recovering' | 'degraded' | 'paused' | 'unknown'

// The pill prefers the confirmed state from the API and falls back to the
// newest probe outcome when it is absent (older servers, no probe yet).
export function deriveMonitorState(
  monitor: Pick<Monitor, 'enabled' | 'state'>,
  results?: MonitorResults,
): MonitorState {
  if (!monitor.enabled) return 'paused'
  if (monitor.state) return monitor.state
  const raw = results?.raw ?? []
  if (raw.length === 0) return 'unknown'
  const newest = raw.reduce((a, b) => (b.at > a.at ? b : a))
  return newest.ok ? 'up' : 'down'
}

export function lastCheckAt(results?: MonitorResults): number | undefined {
  const raw = results?.raw ?? []
  if (raw.length === 0) return undefined
  return Math.max(...raw.map((r) => r.at))
}

export function uptimePercent(results?: MonitorResults): number | undefined {
  const rollup = results?.rollup ?? []
  const total = rollup.reduce((n, r) => n + r.total, 0)
  if (total === 0) return undefined
  const failed = rollup.reduce((n, r) => n + r.failed, 0)
  return ((total - failed) / total) * 100
}

export function monitorTarget(m: Monitor): string {
  if (m.kind === 'heartbeat') return ''
  if (m.kind === 'tcp' || m.kind === 'tls')
    return `${m.host ?? ''}:${m.port ?? ''}`
  return m.url ?? ''
}

export function suggestMonitorName(url: string): string {
  try {
    return new URL(url).hostname
  } catch {
    return ''
  }
}
