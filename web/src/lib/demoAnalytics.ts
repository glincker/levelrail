import type { DemoInfo } from './demoStore'

export type DemoEventName =
  | 'demo_session_started'
  | 'demo_route_viewed'
  | 'demo_readonly_blocked'
  | 'demo_install_command_copied'
  | 'demo_github_click'

type Props = Record<string, string>

interface PostHogLike {
  init: (token: string, options: Record<string, unknown>) => void
  register: (props: Props) => void
  capture: (event: string, props?: Props) => void
}

declare global {
  interface Window {
    posthog?: PostHogLike
    dataLayer?: Record<string, unknown>[]
  }
}

// Not on DemoInfo so the GTM provider stays independent of the PostHog one.
type GtmPayload = { gtm?: { id: string } }

const GTM_ID = /^GTM-[A-Z0-9]{4,12}$/
let gtmActive = false

let client: PostHogLike | null = null
let loading: Promise<void> | null = null
let utm: Props = {}

export function doNotTrackEnabled(
  nav: Pick<Navigator, 'doNotTrack'> | undefined,
): boolean {
  const v = nav?.doNotTrack
  return v === '1' || v === 'yes'
}

export function readUtm(search: string): Props {
  const params = new URLSearchParams(search)
  const out: Props = {}
  for (const key of ['utm_source', 'utm_medium', 'utm_campaign']) {
    const v = params.get(key)
    if (v) out[key] = v.slice(0, 100)
  }
  return out
}

// Route patterns only: concrete ids in a path segment never leave the browser.
export function apiRoutePattern(path: string): string {
  const bare = path.split('?', 1).join('')
  return bare
    .split('/')
    .map((seg) => (/^\d+$|^[\w-]{16,}$/.test(seg) ? ':id' : seg))
    .join('/')
}

function loadScript(host: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const el = document.createElement('script')
    el.async = true
    el.src = `${host}/static/array.js`
    el.onload = () => {
      resolve()
    }
    el.onerror = () => {
      reject(new Error('analytics script failed'))
    }
    document.head.appendChild(el)
  })
}

// Standard external loader: no inline script, so the CSP needs no unsafe-inline.
function initGtm(id: string): void {
  if (gtmActive || !GTM_ID.test(id)) return
  gtmActive = true
  window.dataLayer = window.dataLayer || []
  window.dataLayer.push({ 'gtm.start': Date.now(), event: 'gtm.js' })
  const el = document.createElement('script')
  el.async = true
  el.src = `https://www.googletagmanager.com/gtm.js?id=${encodeURIComponent(id)}`
  document.head.appendChild(el)
}

/** initDemoAnalytics loads each opted-in provider lazily, unless the visitor sends Do Not Track. */
export function initDemoAnalytics(info: DemoInfo): Promise<void> {
  if (info.enabled && typeof document !== 'undefined') {
    const g = (info as DemoInfo & GtmPayload).gtm
    if (g && !doNotTrackEnabled(navigator) && !gtmActive) {
      utm = readUtm(window.location.search)
      initGtm(g.id)
      if (gtmActive) captureDemoEvent('demo_session_started')
    }
  }
  if (loading) return loading
  const a = info.analytics
  if (
    !info.enabled ||
    !a ||
    a.provider !== 'posthog' ||
    typeof document === 'undefined'
  ) {
    return Promise.resolve()
  }
  if (doNotTrackEnabled(navigator)) return Promise.resolve()
  utm = readUtm(window.location.search)
  loading = loadScript(a.host)
    .then(() => {
      const ph = window.posthog
      if (!ph) return
      ph.init(a.token, {
        api_host: a.host,
        persistence: 'memory',
        autocapture: false,
        capture_pageview: false,
        capture_pageleave: false,
        disable_session_recording: true,
        respect_dnt: true,
        loaded: (p: PostHogLike) => {
          p.register({ surface: 'demo' })
        },
      })
      client = ph
      captureDemoEvent('demo_session_started')
    })
    .catch(() => undefined)
  return loading
}

export function captureDemoEvent(name: DemoEventName, props: Props = {}): void {
  const merged = { ...utm, ...props }
  client?.capture(name, merged)
  if (gtmActive) {
    window.dataLayer = window.dataLayer || []
    window.dataLayer.push({ event: name, surface: 'demo', ...merged })
  }
}

export function resetDemoAnalyticsForTest(): void {
  client = null
  loading = null
  gtmActive = false
  delete window.dataLayer
  utm = {}
}
