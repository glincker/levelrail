export interface SleepTimelineEvent {
  kind: string
  at: string
}

export interface SleepSegment {
  asleep: boolean
  startPct: number
  widthPct: number
}

/** buildSleepSegments splits [now - windowMs, now] into asleep and awake runs, as percentages of the window. */
export function buildSleepSegments(
  events: readonly SleepTimelineEvent[],
  nowMs: number,
  windowMs: number,
  currentSinceMs: number | null,
): SleepSegment[] {
  const start = nowMs - windowMs
  const state = events
    .filter((e) => e.kind === 'slept' || e.kind === 'woke')
    .map((e) => ({ asleep: e.kind === 'slept', t: Date.parse(e.at) }))
    .filter((e) => Number.isFinite(e.t) && e.t <= nowMs)
    .sort((a, b) => a.t - b.t)
  const inWindow = state.filter((e) => e.t >= start)

  // Without an event before the window, the first in-window event tells what came before it.
  const first = inWindow[0]
  const before = state[state.length - 1]
  let asleep = first
    ? !first.asleep
    : (before?.asleep ?? currentSinceMs !== null)

  const out: SleepSegment[] = []
  let cursor = start
  const push = (until: number) => {
    if (until <= cursor) return
    const last = out[out.length - 1]
    const startPct = ((cursor - start) / windowMs) * 100
    const widthPct = ((until - cursor) / windowMs) * 100
    if (last && last.asleep === asleep) last.widthPct += widthPct
    else out.push({ asleep, startPct, widthPct })
    cursor = until
  }
  for (const e of inWindow) {
    push(e.t)
    asleep = e.asleep
  }
  push(nowMs)
  return out
}

/** formatSleepDuration renders seconds as "2h 14m", "3d 4h" or "45s". */
export function formatSleepDuration(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds))
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ${m % 60}m`
  return `${Math.floor(h / 24)}d ${h % 24}h`
}
