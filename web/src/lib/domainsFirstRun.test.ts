import { describe, expect, it } from 'vitest'
import { firstRunSteps } from './domainsFirstRun'

const cur = (i: Parameters<typeof firstRunSteps>[0]) =>
  firstRunSteps(i).find((s) => s.current)?.id

describe('firstRunSteps', () => {
  it('starts at deploy with nothing set up', () => {
    expect(cur({ appCount: 0, domainCount: 0, httpsOff: true })).toBe('deploy')
  })
  it('moves to the domain step once an app exists', () => {
    const steps = firstRunSteps({ appCount: 1, domainCount: 0, httpsOff: true })
    expect(steps[0]?.done).toBe(true)
    expect(cur({ appCount: 1, domainCount: 0, httpsOff: true })).toBe('domain')
  })
  it('moves to HTTPS once a domain is pending', () => {
    expect(cur({ appCount: 1, domainCount: 1, httpsOff: true })).toBe('https')
  })
  it('has no current step when HTTPS is on', () => {
    const steps = firstRunSteps({
      appCount: 1,
      domainCount: 1,
      httpsOff: false,
    })
    expect(steps.every((s) => s.done)).toBe(true)
    expect(steps.some((s) => s.current)).toBe(false)
  })
})
