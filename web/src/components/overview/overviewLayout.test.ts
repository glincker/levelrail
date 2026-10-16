import { describe, expect, it } from 'vitest'
import {
  computeOverviewLayout,
  type OverviewLayoutInput,
} from './overviewLayout'

const healthy: OverviewLayoutInput = {
  attentionCount: 0,
  hasTraffic: true,
  trafficKnown: true,
  suspended: false,
  isStatic: false,
  setupRemaining: 0,
  setupDismissed: false,
}

describe('computeOverviewLayout', () => {
  it('healthy with traffic: access then metrics, no setup', () => {
    const l = computeOverviewLayout(healthy)
    expect(l.sections).toEqual([
      'access',
      'kpis',
      'activity',
      'health',
      'details',
    ])
    expect(l.traffic).toBe('full')
    expect(l.setup).toBe('hidden')
    expect(l.pairAttentionWithAccess).toBe(false)
  })

  it('healthy without traffic collapses the traffic block and sinks setup', () => {
    const l = computeOverviewLayout({
      ...healthy,
      hasTraffic: false,
      setupRemaining: 2,
    })
    expect(l.traffic).toBe('hint')
    expect(l.setup).toBe('collapsed')
    expect(l.sections.indexOf('setup')).toBeGreaterThan(
      l.sections.indexOf('kpis'),
    )
  })

  it('a failing deploy or crashloop leads the page', () => {
    for (const attentionCount of [1, 3]) {
      const l = computeOverviewLayout({ ...healthy, attentionCount })
      expect(l.sections[0]).toBe('attention')
      expect(l.sections.indexOf('health')).toBeLessThan(
        l.sections.indexOf('activity'),
      )
      expect(l.pairAttentionWithAccess).toBe(true)
    }
  })

  it('static sites hide container metrics', () => {
    expect(
      computeOverviewLayout({ ...healthy, isStatic: true }).containerMetrics,
    ).toBe(false)
  })

  it('stopped apps hide metrics and traffic', () => {
    const l = computeOverviewLayout({ ...healthy, suspended: true })
    expect(l.containerMetrics).toBe(false)
    expect(l.traffic).toBe('hidden')
  })

  it('a dismissed setup never shows', () => {
    const l = computeOverviewLayout({
      ...healthy,
      setupRemaining: 3,
      setupDismissed: true,
    })
    expect(l.sections).not.toContain('setup')
  })

  it('keeps full traffic while it is still loading', () => {
    expect(
      computeOverviewLayout({
        ...healthy,
        hasTraffic: false,
        trafficKnown: false,
      }).traffic,
    ).toBe('full')
  })
})
