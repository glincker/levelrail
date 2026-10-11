import type { TimelineEvent, TimelineSeverity } from '../types/investigate'

const SEVERITY_RANK: Record<TimelineSeverity, number> = {
  critical: 0,
  warning: 1,
  info: 2,
}

// sortTimeline orders events oldest first so a reader follows cause to
// effect. Same instant: the more severe event first, then input order.
export function sortTimeline(
  events: readonly TimelineEvent[],
): TimelineEvent[] {
  return events
    .map((e, i) => ({ e, i, t: Date.parse(e.at) }))
    .sort((a, b) => {
      if (a.t !== b.t) {
        return a.t - b.t
      }
      const sev = SEVERITY_RANK[a.e.severity] - SEVERITY_RANK[b.e.severity]
      return sev !== 0 ? sev : a.i - b.i
    })
    .map((x) => x.e)
}
