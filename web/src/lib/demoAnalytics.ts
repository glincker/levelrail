import type { DemoInfo } from './demoStore'

export type DemoEventName =
  | 'demo_landing_viewed'
  | 'demo_session_started'
  | 'demo_route_viewed'
  | 'demo_readonly_blocked'
  | 'demo_install_command_copied'
  | 'demo_github_click'
  | 'demo_tour_step'

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

const GTM_ID = /^GTM-[A-Z0-9]{4,12}$/
let gtmActive = false

let client: PostHogLike | null = null
let loading: Promise<void> | null = null
let utm: Props = {}
let gtmLandingSent = false
let phLandingSent = false

export function doNotTrackEnabled(
  nav: Pick<Navigator, 'doNotTrack'> | undefined,
): boolean {
  const v = nav?.doNotTrack
  return v === '1' || v === 'yes'
}

export function globalPrivacyControlEnabled(
  nav: { globalPrivacyControl?: boolean } | undefined,
): boolean {
  return nav?.globalPrivacyControl === true
}

function optedOut(): boolean {
  return doNotTrackEnabled(navigator) || globalPrivacyControlEnabled(navigator)
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

/** initDemoAnalytics loads each opted-in provider lazily, unless the visitor sends Do Not Track or Global Privacy Control. It is idempotent and emits demo_landing_viewed once per page load per provider. */
export function initDemoAnalytics(info: DemoInfo): Promise<void> {
  if (info.enabled && typeof document !== 'undefined') {
    const g = info.gtm
    if (g && !optedOut() && !gtmActive) {
      utm = readUtm(window.location.search)
      initGtm(g.id)
      if (gtmActive && !gtmLandingSent) {
        gtmLandingSent = true
        pushDataLayer('demo_landing_viewed', {})
      }
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
  if (optedOut()) return Promise.resolve()
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
      if (!phLandingSent) {
        phLandingSent = true
        ph.capture('demo_landing_viewed', { ...utm })
      }
    })
    .catch(() => undefined)
  return loading
}

function pushDataLayer(name: DemoEventName, props: Props): void {
  window.dataLayer = window.dataLayer || []
  window.dataLayer.push({ event: name, surface: 'demo', ...utm, ...props })
}

export function captureDemoEvent(name: DemoEventName, props: Props = {}): void {
  client?.capture(name, { ...utm, ...props })
  if (gtmActive) pushDataLayer(name, props)
}

export function resetDemoAnalyticsForTest(): void {
  client = null
  loading = null
  gtmActive = false
  gtmLandingSent = false
  phLandingSent = false
  delete window.dataLayer
  utm = {}
}
