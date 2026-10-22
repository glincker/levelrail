import { describe, expect, it } from 'vitest'
import { metricsEmptyReason, panelsForEngine } from './metricPanels'

describe('metricPanels', () => {
  it('drops the disk panel for in-memory engines', () => {
    expect(panelsForEngine(true)).toEqual(['memory', 'cpu', 'network', 'disk'])
    expect(panelsForEngine(false)).toEqual(['memory', 'cpu', 'network'])
  })

  it('explains why there is nothing to chart', () => {
    expect(metricsEmptyReason({ suspended: true })).toBe('stopped')
    expect(metricsEmptyReason({ provision_state: 'pending' })).toBe(
      'provisioning',
    )
    expect(metricsEmptyReason({})).toBeNull()
  })
})
