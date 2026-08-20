// formatDeployDuration renders the wall-clock time a deploy attempt took,
// from started_at to finished_at. A still-running attempt (no
// finished_at) has no fixed duration to show yet, so it renders as
// 'Running' rather than a live-updating counter: DeployAttemptsList is a
// list, not a focused live view (that's LogTerminal's job), and a
// ticking timer per row would re-render the whole list every second for
// a value nobody is watching closely there.
export function formatDeployDuration(
  startedAt: string,
  finishedAt?: string,
): string {
  if (!finishedAt) {
    return 'Running'
  }
  const ms = new Date(finishedAt).getTime() - new Date(startedAt).getTime()
  return formatDurationMs(ms) ?? 'Unknown'
}

// formatDurationMs is the shared h/m/s formatter behind
// formatDeployDuration above and DeployStageTimeline's live-ticking
// elapsed display, so a fixed and a still-counting-up duration render
// with the same rounding and units. Returns null for a negative or
// non-finite input rather than a fabricated string.
export function formatDurationMs(ms: number): string | null {
  if (!Number.isFinite(ms) || ms < 0) {
    return null
  }
  const totalSeconds = Math.round(ms / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60

  if (hours > 0) {
    return `${hours}h ${minutes}m`
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds}s`
  }
  return `${seconds}s`
}
