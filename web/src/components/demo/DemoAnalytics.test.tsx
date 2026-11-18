import { render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  resetDemoAnalyticsForTest,
  initDemoAnalytics,
} from '@/lib/demoAnalytics'
import { setDemoInfo, type DemoInfo } from '@/lib/demoStore'
import { DemoAnalytics } from './DemoAnalytics'

const demo: DemoInfo = { enabled: true, gtm: { id: 'GTM-ABC123' } }

function gtmScripts(): number {
  return document.head.querySelectorAll('script[src*="googletagmanager"]')
    .length
}

describe('DemoAnalytics on the login route', () => {
  afterEach(() => {
    resetDemoAnalyticsForTest()
    setDemoInfo({ enabled: false })
    vi.unstubAllGlobals()
    document.head.innerHTML = ''
  })

  it('loads GTM and emits demo_landing_viewed when the demo is enabled', () => {
    vi.stubGlobal('navigator', { doNotTrack: null })
    setDemoInfo(demo)
    render(<DemoAnalytics />)
    expect(gtmScripts()).toBe(1)
    expect(
      window.dataLayer?.filter((e) => e.event === 'demo_landing_viewed'),
    ).toHaveLength(1)
  })

  it('loads PostHog when configured', () => {
    vi.stubGlobal('navigator', { doNotTrack: null })
    setDemoInfo({
      enabled: true,
      analytics: { provider: 'posthog', host: 'https://ph.test', token: 't' },
    })
    render(<DemoAnalytics />)
    expect(
      document.head.querySelector(
        'script[src="https://ph.test/static/array.js"]',
      ),
    ).not.toBeNull()
  })

  it.each([
    ['Do Not Track', { doNotTrack: '1' }],
    ['Global Privacy Control', { globalPrivacyControl: true }],
  ])('loads nothing with %s', (_n, nav) => {
    vi.stubGlobal('navigator', nav)
    setDemoInfo(demo)
    render(<DemoAnalytics />)
    expect(document.head.querySelector('script')).toBeNull()
  })

  it('loads nothing on a real instance', () => {
    vi.stubGlobal('navigator', { doNotTrack: null })
    setDemoInfo({ enabled: false, gtm: { id: 'GTM-ABC123' } })
    render(<DemoAnalytics />)
    expect(document.head.querySelector('script')).toBeNull()
  })

  it('does not load twice when DemoChrome inits after sign-in', () => {
    vi.stubGlobal('navigator', { doNotTrack: null })
    setDemoInfo(demo)
    render(<DemoAnalytics />)
    // DemoChrome runs this same call once the visitor is signed in.
    void initDemoAnalytics(demo)
    expect(gtmScripts()).toBe(1)
    expect(
      window.dataLayer?.filter((e) => e.event === 'demo_landing_viewed'),
    ).toHaveLength(1)
  })
})
