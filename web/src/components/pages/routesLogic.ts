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
  force?: boolean
}

export interface MatchCapture {
  name: string
  value: string
}

export interface MatchResult {
  index: number
  status: number
  target: string
  kind: RuleKind
  from: string
  force: boolean
  /** Request path after the server's clean-up, which is what the rules saw. */
  path: string
  captures: MatchCapture[]
  /** A non-forced rule yields to a real file at the path, which the browser cannot see. */
  yieldsToFile: boolean
}

export interface TestInput {
  path: string
  query: string
}

/** Tester input to what the server sees: leading slash added, fragment dropped, ?query split off. */
export function normalizeTestInput(
  pathInput: string,
  queryInput: string,
): TestInput {
  const noFrag = (pathInput.split('#')[0] ?? '').trim()
  const [rawPath = '', ...rest] = noFrag.split('?')
  const q = (queryInput.trim() || rest.join('?')).replace(/^\?/, '')
  const path = rawPath.trim()
  if (path === '') return { path: '', query: q }
  return { path: path.startsWith('/') ? path : `/${path}`, query: q }
}

/** Same cap as the server, which answers 414 beyond it. */
export const MAX_REQUEST_PATH = 2048

const SPLAT = 'splat'
const PLACEHOLDER = /:([A-Za-z_][A-Za-z0-9_]*)/g
const NAME = /^[A-Za-z_][A-Za-z0-9_]*$/

function escapeRe(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/** Mirrors the server's path clean-up: dot segments resolved, one trailing slash kept. */
export function cleanRequestPath(p: string): string {
  const out: string[] = []
  for (const seg of p.split('/')) {
    if (seg === '' || seg === '.') continue
    if (seg === '..') out.pop()
    else out.push(seg)
  }
  const joined = '/' + out.join('/')
  return p.endsWith('/') && joined !== '/' ? joined + '/' : joined
}

/** Server pattern rules: case-insensitive, optional trailing slash, trailing * is an optional splat. */
function compile(from: string): { re: RegExp; names: string[] } | null {
  const trimmed = from.endsWith('/') ? from.slice(0, -1) : from
  const segs = trimmed === '' ? [] : trimmed.slice(1).split('/')
  const names: string[] = []
  let body = ''
  for (let i = 0; i < segs.length; i++) {
    const seg = segs[i] ?? ''
    if (seg === '') return null
    if (seg === '*') {
      if (i !== segs.length - 1) return null
      body += '(?:/([\\s\\S]*))?'
      names.push(SPLAT)
    } else if (seg.startsWith(':')) {
      const name = seg.slice(1)
      if (!NAME.test(name) || name === SPLAT || names.includes(name)) {
        return null
      }
      body += '/([^/]+)'
      names.push(name)
    } else if (/[*:]/.test(seg)) {
      return null
    } else {
      body += '/' + escapeRe(seg)
    }
  }
  body += segs.length === 0 ? '/' : '/?'
  return { re: new RegExp(`^${body}$`, 'i'), names }
}

const KEEP = /[!'()*]/g
const pct = (c: string) => '%' + c.charCodeAt(0).toString(16).toUpperCase()

function pathEscape(seg: string): string {
  return encodeURIComponent(seg)
    .replace(/%(24|26|2B|3D|3A|40)/g, (_, h: string) =>
      String.fromCharCode(parseInt(h, 16)),
    )
    .replace(KEEP, pct)
}

function queryEscape(v: string): string {
  return encodeURIComponent(v).replace(KEEP, pct).replace(/%20/g, '+')
}

/** Re-escapes captured values like the server so a decoded control char never reaches a Location. */
function substitute(to: string, caps: Map<string, string>): string {
  const q = to.indexOf('?')
  const fill = (s: string, query: boolean) =>
    s.replace(PLACEHOLDER, (tok: string, name: string) => {
      const v = caps.get(name)
      if (v === undefined) return tok
      return query ? queryEscape(v) : v.split('/').map(pathEscape).join('/')
    })
  if (q < 0) return fill(to, false)
  return fill(to.slice(0, q), false) + '?' + fill(to.slice(q + 1), true)
}

function guardLocation(loc: string): string {
  return loc.startsWith('/') ? '/' + loc.replace(/^[/\\]+/, '') : loc
}

const stripSlash = (s: string) => (s.endsWith('/') ? s.slice(0, -1) : s)

/**
 * First-match-wins client matcher over rules in precedence order, for the live tester.
 * Mirrors internal/staticroutes: the query never takes part in matching, it is only carried
 * onto redirect targets. Returns null for no match or a path past the server cap.
 */
export function matchPath(
  rules: readonly MatchRule[],
  rawPath: string,
  query = '',
): MatchResult | null {
  const path = cleanRequestPath(rawPath)
  if (path.length > MAX_REQUEST_PATH) return null
  for (let i = 0; i < rules.length; i++) {
    const r = rules[i]
    if (!r || !r.from.startsWith('/') || BAD.test(r.from)) continue
    const c = compile(r.from)
    if (!c) continue
    const m = c.re.exec(path)
    if (!m) continue
    const caps = new Map<string, string>()
    c.names.forEach((n, j) => caps.set(n, m[j + 1] ?? ''))
    const proxy = parseProxy(r.to) !== null
    let target = substitute(r.to, caps)
    if (r.status >= 300 && r.status < 400 && !proxy) {
      const self =
        target.startsWith('/') &&
        !target.includes('?') &&
        stripSlash(target) === stripSlash(path)
      if (self) continue
      const withQ =
        query && !target.includes('?') ? `${target}?${query}` : target
      target = guardLocation(withQ)
    }
    return {
      index: i,
      status: r.status,
      target,
      kind: inferKind(r.to, r.status),
      from: r.from,
      force: Boolean(r.force),
      path,
      captures: c.names.map((n) => ({ name: n, value: caps.get(n) ?? '' })),
      yieldsToFile: !r.force && !proxy,
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
