import type { DeployAttempt } from '../types/deployAttempt'

export type HealthWindowKind = 'failed_deploy' | 'crashloop'

export interface TimelineDeploy {
  id: string
  t: number
  pos: number
  status: DeployAttempt['status']
  label: string
}

export interface TimelineRestartBucket {
  key: string
  t: number
  pos: number
  count: number
  label: string
}

export interface TimelineWindow {
  key: string
  kind: HealthWindowKind
  startPos: number
  endPos: number
  label: string
}

export interface HealthTimeline {
  deploys: TimelineDeploy[]
  restarts: TimelineRestartBucket[]
  windows: TimelineWindow[]
}

export interface TimelineInput {
  attempts: DeployAttempt[]
  restartTimes: number[]
  from: number
  to: number
}

export const RESTART_BUCKETS = 48
export const CRASHLOOP_MIN_RESTARTS = 3
export const CRASHLOOP_MAX_GAP_MS = 15 * 60 * 1000
const FAILED_MIN_WINDOW_MS = 2 * 60 * 1000

export function positionOf(t: number, from: number, to: number): number {
  if (to <= from) {
    return 0
  }
  return Math.min(1, Math.max(0, (t - from) / (to - from)))
}

export function formatTimelineTime(t: number): string {
  return new Date(t).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function deployLabel(a: DeployAttempt, t: number): string {
  let what = 'Deploy succeeded'
  if (a.status === 'failed') {
    what = 'Failed deploy'
  } else if (a.status === 'running') {
    what = 'Deploy running'
  }
  const sha = a.commit_sha ? ` (${a.commit_sha.slice(0, 7)})` : ''
  return `${what}${sha} at ${formatTimelineTime(t)}`
}

export function buildDeployMarkers(
  attempts: DeployAttempt[],
  from: number,
  to: number,
): TimelineDeploy[] {
  const out: TimelineDeploy[] = []
  for (const a of attempts) {
    const t = Date.parse(a.started_at)
    if (Number.isNaN(t) || t < from || t > to) {
      continue
    }
    out.push({
      id: a.id,
      t,
      pos: positionOf(t, from, to),
      status: a.status,
      label: deployLabel(a, t),
    })
  }
  return out.sort((x, y) => x.t - y.t)
}

export function bucketRestarts(
  times: number[],
  from: number,
  to: number,
  buckets: number = RESTART_BUCKETS,
): TimelineRestartBucket[] {
  const span = to - from
  if (span <= 0 || buckets <= 0) {
    return []
  }
  const width = span / buckets
  const counts = new Map<number, number>()
  for (const t of times) {
    if (t < from || t > to) {
      continue
    }
    const idx = Math.min(buckets - 1, Math.floor((t - from) / width))
    counts.set(idx, (counts.get(idx) ?? 0) + 1)
  }
  return [...counts.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([idx, count]) => {
      const t = from + idx * width + width / 2
      const noun = count === 1 ? 'restart' : 'restarts'
      return {
        key: `restart-${idx}`,
        t,
        pos: positionOf(t, from, to),
        count,
        label: `${count} container ${noun} around ${formatTimelineTime(t)}`,
      }
    })
}

export function failedDeployWindows(
  attempts: DeployAttempt[],
  from: number,
  to: number,
): TimelineWindow[] {
  const out: TimelineWindow[] = []
  for (const a of attempts) {
    if (a.status !== 'failed') {
      continue
    }
    const start = Date.parse(a.started_at)
    if (Number.isNaN(start)) {
      continue
    }
    const parsedEnd = a.finished_at ? Date.parse(a.finished_at) : NaN
    const end = Math.max(
      Number.isNaN(parsedEnd) ? start : parsedEnd,
      start + FAILED_MIN_WINDOW_MS,
    )
    if (end < from || start > to) {
      continue
    }
    out.push({
      key: `failed-${a.id}`,
      kind: 'failed_deploy',
      startPos: positionOf(start, from, to),
      endPos: positionOf(end, from, to),
      label: `Failed deploy ${a.id.slice(0, 8)} at ${formatTimelineTime(start)}`,
    })
  }
  return out
}

export function crashloopWindows(
  times: number[],
  from: number,
  to: number,
): TimelineWindow[] {
  const sorted = times.filter((t) => t >= from && t <= to).sort((a, b) => a - b)
  const out: TimelineWindow[] = []
  let cluster: number[] = []
  const flush = () => {
    const first = cluster[0]
    const last = cluster[cluster.length - 1]
    if (
      first !== undefined &&
      last !== undefined &&
      cluster.length >= CRASHLOOP_MIN_RESTARTS
    ) {
      out.push({
        key: `crashloop-${first}`,
        kind: 'crashloop',
        startPos: positionOf(first, from, to),
        endPos: positionOf(last, from, to),
        label: `Crashloop: ${cluster.length} restarts from ${formatTimelineTime(first)}`,
      })
    }
    cluster = []
  }
  for (const t of sorted) {
    const prev = cluster[cluster.length - 1]
    if (prev !== undefined && t - prev > CRASHLOOP_MAX_GAP_MS) {
      flush()
    }
    cluster.push(t)
  }
  flush()
  return out
}

export function buildHealthTimeline(input: TimelineInput): HealthTimeline {
  const { attempts, restartTimes, from, to } = input
  return {
    deploys: buildDeployMarkers(attempts, from, to),
    restarts: bucketRestarts(restartTimes, from, to),
    windows: [
      ...failedDeployWindows(attempts, from, to),
      ...crashloopWindows(restartTimes, from, to),
    ].sort((a, b) => a.startPos - b.startPos),
  }
}
