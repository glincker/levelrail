import { describe, expect, it } from 'vitest'
import { supportDetails } from './supportDetails'

describe('supportDetails', () => {
  it('lists route, version and browser', () => {
    const out = supportDetails('Support details', '/apps/my-app', '1.5.9')
    expect(out.split('\n')[0]).toBe('Support details')
    expect(out).toContain('version: 1.5.9')
    expect(out).toContain('browser: ')
    expect(out).toContain('route: /apps/my-app')
  })
  it('omits version when unknown', () => {
    expect(supportDetails('h', '/', undefined)).not.toContain('version:')
  })
})
