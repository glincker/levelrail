import type { DraftRedirect } from './rulesDraft'

export type RuleKind = 'redirect' | 'rewrite' | 'proxy' | 'status'
export type TargetState = 'ok' | 'missing' | 'self'

const PROXY_PREFIX = 'app://'

export interface ProxyTarget {
  app: string
  path: string
}

export function parseProxy(to: string): ProxyTarget | null {
  const t = to.trim()
  if (!t.startsWith(PROXY_PREFIX)) return null
  const rest = t.slice(PROXY_PREFIX.length)
  const slash = rest.indexOf('/')
  if (slash < 0) return { app: rest, path: '' }
  return { app: rest.slice(0, slash), path: rest.slice(slash) }
}

export function buildProxy(app: string, path: string): string {
  return `${PROXY_PREFIX}${app}${path}`
}

/** Kind is derived from the target and status, so it never drifts from the stored rule. */
export function inferKind(to: string, status: number): RuleKind {
  if (parseProxy(to)) return 'proxy'
  if (status === 200) return 'rewrite'
  if (status >= 400) return 'status'
  return 'redirect'
}

/** Status a row takes when its kind is switched in the editor. */
export function statusForKind(kind: RuleKind, current: string): string {
  if (kind === 'proxy' || kind === 'rewrite') return '200'
  if (kind === 'status') return /^4\d\d$/.test(current) ? current : '404'
  return /^30\d$/.test(current) ? current : '301'
}

export function targetState(
  to: string,
  self: string,
  apps: readonly string[],
): TargetState | undefined {
  const p = parseProxy(to)
  if (!p) return undefined
  if (p.app === self) return 'self'
  return apps.includes(p.app) ? 'ok' : 'missing'
}

export type RuleProblem =
  | 'fromSlash'
  | 'badChars'
  | 'toEmpty'
  | 'proxyApp'
  | 'proxyAppMissing'
  | 'proxyAppSelf'
  | 'proxyPath'

const BAD = /[{}\\]/

/** First problem for a row, or null. Mirrors the server's checks so errors show before save. */
export function validateRow(
  row: Pick<DraftRedirect, 'from' | 'to' | 'status'>,
  self: string,
  apps: readonly string[],
): RuleProblem | null {
  const from = row.from.trim()
  const to = row.to.trim()
  if (!from.startsWith('/')) return 'fromSlash'
  if (BAD.test(from) || BAD.test(to)) return 'badChars'
  const proxy = parseProxy(to)
  if (proxy) {
    if (!proxy.app) return 'proxyApp'
    if (proxy.app === self) return 'proxyAppSelf'
    if (apps.length > 0 && !apps.includes(proxy.app)) return 'proxyAppMissing'
    if (proxy.path !== '' && !proxy.path.startsWith('/')) return 'proxyPath'
    return null
  }
  if (to === '' && Number(row.status) < 400) return 'toEmpty'
  return null
}

export interface MatchRule {
  from: string
  to: string
  status: number
}

export interface MatchResult {
  index: number
  status: number
  target: string
  kind: RuleKind
}

function escapeRe(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function compile(from: string): { re: RegExp; names: string[] } {
  const names: string[] = []
  const body = from
    .split('/')
    .map((seg) => {
      if (seg === '*') {
        names.push('splat')
        return '(.*)'
      }
      if (seg.startsWith(':') && seg.length > 1) {
        names.push(seg.slice(1))
        return '([^/]+)'
      }
      return escapeRe(seg)
    })
    .join('/')
  return { re: new RegExp(`^${body}$`), names }
}

/** First-match-wins client matcher over rules in precedence order, for the live tester. */
export function matchPath(
  rules: readonly MatchRule[],
  path: string,
): MatchResult | null {
  for (let i = 0; i < rules.length; i++) {
    const r = rules[i]
    if (!r || !r.from.startsWith('/') || BAD.test(r.from)) continue
    const { re, names } = compile(r.from)
    const m = re.exec(path)
    if (!m) continue
    let target = r.to
    names.forEach((n, j) => {
      target = target.split(`:${n}`).join(m[j + 1] ?? '')
    })
    return {
      index: i,
      status: r.status,
      target,
      kind: inferKind(r.to, r.status),
    }
  }
  return null
}

export interface Preset {
  id: 'spa' | 'moved' | 'api'
  from: string
  to: string
  status: string
}

export const PRESETS: readonly Preset[] = [
  { id: 'spa', from: '/*', to: '/index.html', status: '200' },
  { id: 'moved', from: '/blog/*', to: '/news/:splat', status: '301' },
  { id: 'api', from: '/api/*', to: 'app:///:splat', status: '200' },
]
