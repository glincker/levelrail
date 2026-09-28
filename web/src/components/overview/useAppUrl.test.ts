import { describe, expect, it } from 'vitest'
import { appUrlFrom } from './useAppUrl'

describe('appUrlFrom', () => {
  it('prefers the network fallback url over the dashboard host and port', () => {
    const url = appUrlFrom(
      { domains: [] },
      {
        running: true,
        host_port: 8080,
        fallback_url: 'http://1-2-3-4.sslip.io',
      },
      'dash.local',
    )
    expect(url).toBe('http://1-2-3-4.sslip.io')
  })
  it('uses the domain first', () => {
    expect(appUrlFrom({ domains: ['a.dev'] }, undefined, 'h')).toBe(
      'https://a.dev',
    )
  })
})
