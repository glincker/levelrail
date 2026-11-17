import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  apiRoutePattern,
  captureDemoEvent,
  doNotTrackEnabled,
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
    expect(capture).toHaveBeenCalledWith('demo_session_started', {})
    expect(capture).toHaveBeenCalledWith('demo_github_click', {})
  })

  it('keeps ids and queries out of route patterns and reads utm', () => {
    expect(apiRoutePattern('/api/v1/apps/123/deploy?x=1')).toBe(
      '/api/v1/apps/:id/deploy',
    )
    expect(readUtm('?utm_source=hn&foo=1')).toEqual({ utm_source: 'hn' })
  })
})
