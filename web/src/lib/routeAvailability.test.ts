import { describe, expect, it } from 'vitest'
import { hasRoute } from './routeAvailability'

describe('hasRoute', () => {
  it('matches an exact path', () => {
    expect(hasRoute(['/domains', '/dns'], '/dns')).toBe(true)
  })

  it('matches an index route registered with a trailing slash', () => {
    expect(hasRoute(['/domains/'], '/domains')).toBe(true)
  })

  it('does not match a missing route or a mere prefix', () => {
    expect(hasRoute(['/domains'], '/dns')).toBe(false)
    expect(hasRoute(['/dns/$zone'], '/dns')).toBe(false)
  })

  it('handles the root path', () => {
    expect(hasRoute(['/'], '/')).toBe(true)
  })
})
