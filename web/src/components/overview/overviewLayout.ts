export type OverviewSection =
  | 'attention'
  | 'diagnosis'
  | 'access'
  | 'kpis'
  | 'health'
  | 'activity'
  | 'details'
  | 'setup'

export interface OverviewLayoutInput {
  /** Failing signals: failed deploy, crashloop, unhealthy, high errors, cert errors, OOM. */
  attentionCount: number
  /** The single Diagnosis card is showing (failed deploy, crashloop or not ready). */
  diagnosisShown: boolean
  hasTraffic: boolean
  trafficKnown: boolean
  suspended: boolean
  /** Static sites have no container to measure. */
  isStatic: boolean
  setupRemaining: number
  setupDismissed: boolean
}

export interface OverviewLayout {
  sections: OverviewSection[]
  containerMetrics: boolean
  traffic: 'full' | 'hint' | 'hidden'
  setup: 'hidden' | 'collapsed'
}

/** Orders the Overview sections from live data; failing things rise, empty ones sink. */
export function computeOverviewLayout(i: OverviewLayoutInput): OverviewLayout {
  const failing = i.attentionCount > 0
  const containerMetrics = !i.isStatic && !i.suspended
  const traffic: OverviewLayout['traffic'] = i.suspended
    ? 'hidden'
    : i.hasTraffic || !i.trafficKnown
      ? 'full'
      : 'hint'
  const setupVisible = !i.setupDismissed && i.setupRemaining > 0

  const sections: OverviewSection[] = [
    'kpis',
    'activity',
    'access',
    'health',
    'details',
  ]
  if (failing) sections.unshift('attention')
  if (i.diagnosisShown) sections.unshift('diagnosis')
  if (setupVisible) sections.splice(sections.indexOf('details'), 0, 'setup')

  return {
    sections,
    containerMetrics,
    traffic,
    setup: setupVisible ? 'collapsed' : 'hidden',
  }
}
