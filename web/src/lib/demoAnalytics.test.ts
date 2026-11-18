import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  apiRoutePattern,
  captureDemoEvent,
  doNotTrackEnabled,
  globalPrivacyControlEnabled,
  initDemoAnalytics,
  readUtm,
  resetDemoAnalyticsForTest,
} from './demoAnalytics'

const info = {
  enabled: true,
  analytics: {
    provider: 'posthog',
    host: 'https://ph.example.com',
    token: 'phc_x',
  },
}

describe('demoAnalytics', () => {
  afterEach(() => {
    resetDemoAnalyticsForTest()
    vi.unstubAllGlobals()
    document.head.innerHTML = ''
    delete window.posthog
  })

  it('detects Do Not Track', () => {
    expect(doNotTrackEnabled({ doNotTrack: '1' })).toBe(true)
    expect(doNotTrackEnabled({ doNotTrack: '0' })).toBe(false)
  })

  it('detects Global Privacy Control', () => {
    expect(globalPrivacyControlEnabled({ globalPrivacyControl: true })).toBe(
      true,
    )
    expect(globalPrivacyControlEnabled({ globalPrivacyControl: false })).toBe(
      false,
    )
    expect(globalPrivacyControlEnabled({})).toBe(false)
  })

  it('never loads anything with Global Privacy Control', async () => {
    vi.stubGlobal('navigator', { doNotTrack: null, globalPrivacyControl: true })
    await initDemoAnalytics({ ...info, gtm: { id: 'GTM-ABC123' } })
    expect(document.head.querySelector('script')).toBeNull()
    expect(window.dataLayer).toBeUndefined()
  })

  it('never loads the script without a payload or with DNT', async () => {
    await initDemoAnalytics({ enabled: true })
    vi.stubGlobal('navigator', { doNotTrack: '1' })
    await initDemoAnalytics(info)
    expect(document.head.querySelector('script')).toBeNull()
  })

  it('inits cookieless and captures only named events with utm', async () => {
    const capture = vi.fn()
    const init = vi.fn()
    window.posthog = { init, register: vi.fn(), capture }
    vi.stubGlobal('navigator', { doNotTrack: null })
    const p = initDemoAnalytics(info)
    const script = document.head.querySelector('script')
    expect(script?.src).toBe('https://ph.example.com/static/array.js')
    script?.onload?.(new Event('load'))
    await p
    expect(init.mock.calls[0]?.[1]).toMatchObject({
      persistence: 'memory',
      autocapture: false,
      disable_session_recording: true,
      capture_pageview: false,
    })
    captureDemoEvent('demo_github_click')
    captureDemoEvent('demo_tour_step', { step: 'apps' })
    expect(capture).toHaveBeenCalledWith('demo_tour_step', { step: 'apps' })
    expect(capture).toHaveBeenCalledWith('demo_landing_viewed', {})
    expect(capture).not.toHaveBeenCalledWith('demo_session_started', {})
    expect(capture).toHaveBeenCalledWith('demo_github_click', {})
  })

  it('keeps ids and queries out of route patterns and reads utm', () => {
    expect(apiRoutePattern('/api/v1/apps/123/deploy?x=1')).toBe(
      '/api/v1/apps/:id/deploy',
    )
    expect(readUtm('?utm_source=hn&foo=1')).toEqual({ utm_source: 'hn' })
  })
})

describe('demoAnalytics GTM', () => {
  const gtmInfo = { enabled: true, gtm: { id: 'GTM-ABC123' } }
  afterEach(() => {
    resetDemoAnalyticsForTest()
    vi.unstubAllGlobals()
    document.head.innerHTML = ''
  })

  it('does not load with Do Not Track or an invalid id', async () => {
    vi.stubGlobal('navigator', { doNotTrack: '1' })
    await initDemoAnalytics(gtmInfo)
    vi.stubGlobal('navigator', { doNotTrack: '0' })
    await initDemoAnalytics({ enabled: true, gtm: { id: 'bad"id' } })
    expect(document.head.querySelector('script')).toBeNull()
    expect(window.dataLayer).toBeUndefined()
  })

  it('loads once and mirrors events without ids', async () => {
    vi.stubGlobal('navigator', { doNotTrack: null })
    await initDemoAnalytics(gtmInfo)
    await initDemoAnalytics(gtmInfo)
    const scripts = document.head.querySelectorAll('script')
    expect(scripts).toHaveLength(1)
    expect(scripts.item(0)?.src).toBe(
      'https://www.googletagmanager.com/gtm.js?id=GTM-ABC123',
    )
    captureDemoEvent('demo_route_viewed', {
      route: apiRoutePattern('/api/v1/apps/1234567890123456789?x=1'),
    })
    captureDemoEvent('demo_readonly_blocked')
    captureDemoEvent('demo_install_command_copied')
    captureDemoEvent('demo_github_click')
    const events = (window.dataLayer ?? []).filter(
      (e) => typeof e.event === 'string' && e.event.startsWith('demo_'),
    )
    expect(events.map((e) => e.event)).toEqual([
      'demo_landing_viewed',
      'demo_route_viewed',
      'demo_readonly_blocked',
      'demo_install_command_copied',
      'demo_github_click',
    ])
    expect(events.every((e) => e.surface === 'demo')).toBe(true)
    expect(JSON.stringify(events)).not.toMatch(/1234567890|\?x=1/)
  })
})

describe('demoAnalytics landing', () => {
  afterEach(() => {
    resetDemoAnalyticsForTest()
    vi.unstubAllGlobals()
    document.head.innerHTML = ''
    delete window.posthog
  })

  it('sends demo_landing_viewed once to each provider across repeated inits', async () => {
    const capture = vi.fn()
    window.posthog = { init: vi.fn(), register: vi.fn(), capture }
    vi.stubGlobal('navigator', { doNotTrack: null })
    const both = {
      ...info,
      gtm: { id: 'GTM-ABC123' },
    }
    const p = initDemoAnalytics(both)
    void initDemoAnalytics(both)
    document.head
      .querySelector<HTMLScriptElement>('script[src$="array.js"]')
      ?.onload?.(new Event('load'))
    await p
    await initDemoAnalytics(both)
    expect(document.head.querySelectorAll('script')).toHaveLength(2)
    expect(
      capture.mock.calls.filter((c) => c[0] === 'demo_landing_viewed'),
    ).toHaveLength(1)
    const landing = (window.dataLayer ?? []).filter(
      (e) => e.event === 'demo_landing_viewed',
    )
    expect(landing).toHaveLength(1)
  })

  it('loads nothing for a non-demo instance', async () => {
    vi.stubGlobal('navigator', { doNotTrack: null })
    await initDemoAnalytics({
      enabled: false,
      gtm: { id: 'GTM-ABC123' },
      analytics: { provider: 'posthog', host: 'https://x.test', token: 't' },
    })
    expect(document.head.querySelector('script')).toBeNull()
    expect(window.dataLayer).toBeUndefined()
  })
})
