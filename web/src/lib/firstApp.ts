import type { AppListEntry } from '../types/appDetail'
import type { AppNetwork } from '../queries/appNetwork'

export const APP_POLL_INTERVAL_MS = 3_000
export const APP_POLL_BUDGET_MS = 10 * 60_000
export const APP_SLOW_AFTER_MS = 2 * 60_000

/** SAMPLE_APP is the one-click first deploy: small, public, and serves 200 on /. */
export const SAMPLE_APP = {
  name: 'sample-app',
  image: 'nginx:alpine',
  port: 80,
  readinessPath: '/',
} as const

/** pollInterval returns intervalMs while polling should continue, false once done or past maxMs since startedAt. */
export function pollInterval(
  done: boolean,
  startedAt: number,
  now: number,
  intervalMs: number,
  maxMs: number,
): number | false {
  if (done) return false
  if (now - startedAt >= maxMs) return false
  return intervalMs
}

export type AppPhase = 'none' | 'deploying' | 'slow' | 'live' | 'failed'

/** pickTrackedApp prefers the sample app, else the first app. */
export function pickTrackedApp(
  apps: AppListEntry[],
  preferredName: string,
): AppListEntry | undefined {
  return apps.find((a) => a.name === preferredName) ?? apps[0]
}

/** firstAppPhase maps an app's status summary plus elapsed time onto deploy phases. */
export function firstAppPhase(
  app: AppListEntry | undefined,
  trackingSince: number,
  now: number,
  slowAfterMs: number,
): AppPhase {
  if (!app) return 'none'
  if (app.status.variant === 'success') return 'live'
  if (app.status.variant === 'destructive') return 'failed'
  return now - trackingSince >= slowAfterMs ? 'slow' : 'deploying'
}

/** liveUrl picks the best address to open a live app at. */
export function liveUrl(
  app: AppListEntry,
  network: AppNetwork | undefined,
  hostname: string,
): string | undefined {
  const [domain] = app.domains ?? []
  if (domain) return `https://${domain}`
  if (network?.fallback_url) return network.fallback_url
  const loopbackOnly = app.bind_address === 'private' || app.bind_address === ''
  const browserOnHost = hostname === 'localhost' || hostname === '127.0.0.1'
  if (network?.host_port && (!loopbackOnly || browserOnHost)) {
    return `http://${hostname}:${network.host_port}`
  }
  return undefined
}
