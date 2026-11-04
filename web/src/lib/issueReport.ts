// Builds a prefilled GitHub issue-form URL from non-sensitive page context.
// Pure: no network, no storage. Field ids match .github/ISSUE_TEMPLATE/*.yml.

export type IssueTemplate =
  | 'bug_report'
  | 'docs_problem'
  | 'migration_problem'
  | 'feature_request'
  | 'template_request'

export type IssueSource = 'help' | 'error' | 'feature'

export interface IssueDraft {
  template: IssueTemplate
  title: string
  fields: Record<string, string>
}

// Mirrors the component dropdown in bug_report.yml; a test fails on drift.
export const COMPONENT_OPTIONS = {
  control: 'Control plane (reconciler, store, API)',
  node: 'Node agent',
  ingress: 'Ingress / TLS (Caddy)',
  build: 'Build (BuildKit, Railpack)',
  secrets: 'Secrets',
  frontend: 'Frontend / dashboard',
  cli: 'CLI',
  other: 'Other / not sure',
} as const

// Only used when VITE_ISSUE_URL_MAX_CHARS is unset or invalid. Browsers and
// GitHub accept long URLs but proxies and servers often cut near 8000.
export const DEFAULT_ISSUE_URL_MAX_CHARS = 6000

export function issueUrlMaxChars(
  raw: unknown = import.meta.env.VITE_ISSUE_URL_MAX_CHARS,
): number {
  const n = Number(raw)
  return Number.isInteger(n) && n > 200 ? n : DEFAULT_ISSUE_URL_MAX_CHARS
}

export type FieldId =
  | 'version'
  | 'component'
  | 'actual'
  | 'context'
  | 'page'
  | 'problem'
  | 'failed'
  | 'config'
  | 'solution'
  | 'service'
  | 'upstream'
  | 'ports'

export interface FieldSpec {
  id: FieldId
  kind: 'input' | 'textarea' | 'select'
}

export const TEMPLATE_FIELDS: Record<IssueTemplate, FieldSpec[]> = {
  bug_report: [
    { id: 'version', kind: 'input' },
    { id: 'component', kind: 'select' },
    { id: 'actual', kind: 'textarea' },
    { id: 'context', kind: 'textarea' },
  ],
  docs_problem: [
    { id: 'page', kind: 'input' },
    { id: 'problem', kind: 'textarea' },
  ],
  migration_problem: [
    { id: 'version', kind: 'input' },
    { id: 'failed', kind: 'textarea' },
    { id: 'config', kind: 'textarea' },
  ],
  feature_request: [
    { id: 'problem', kind: 'textarea' },
    { id: 'solution', kind: 'textarea' },
    { id: 'context', kind: 'textarea' },
  ],
  template_request: [
    { id: 'service', kind: 'input' },
    { id: 'upstream', kind: 'input' },
    { id: 'ports', kind: 'textarea' },
  ],
}

// The field the user is expected to type in; always shown.
export const MAIN_FIELD: Record<IssueTemplate, string> = {
  bug_report: 'actual',
  docs_problem: 'problem',
  migration_problem: 'failed',
  feature_request: 'problem',
  template_request: 'service',
}

export const TITLE_PREFIX: Record<IssueTemplate, string> = {
  bug_report: 'bug: ',
  docs_problem: 'docs: ',
  migration_problem: 'migration: ',
  feature_request: 'feat: ',
  template_request: 'template: ',
}

// Cut first, from the end, in this order.
const TRUNCATE_ORDER = ['logs', 'context', 'actual', 'config', 'problem']
// Dropped first to last when truncation is not enough.
const DROP_ORDER = [
  'context',
  'logs',
  'config',
  'solution',
  'component',
  'version',
]
const ELLIPSIS = '\n[...truncated]'

const SECRET_PATTERNS: Array<[RegExp, string]> = [
  [
    /\b[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b/g,
    '[redacted]',
  ],
  [/\bBearer\s+[A-Za-z0-9._~+/=-]+/gi, 'Bearer [redacted]'],
  [
    /\b([A-Za-z0-9_]*(?:token|secret|password|passwd|api[_-]?key|authorization|cookie|session)[A-Za-z0-9_]*)\s*[:=]\s*\S+/gi,
    '$1=[redacted]',
  ],
  [/\b[A-Za-z0-9+/_-]{32,}={0,2}/g, '[redacted]'],
  [/[\w.+-]+@[\w-]+(?:\.[\w-]+)+/g, '[email]'],
  [/(https?:\/\/)[^\s/@]+:[^\s/@]+@/gi, '$1[redacted]@'],
  [/(\bhttps?:\/\/[^\s?#]+)[?#]\S*/gi, '$1'],
]

export function redactText(input: string): string {
  let out = input
  for (const [re, rep] of SECRET_PATTERNS) out = out.replace(re, rep)
  return out
}

/** Route id or path to a template without query, hash, layout segments. */
export function routeTemplate(routeIdOrPath: string): string {
  const noQuery = routeIdOrPath.split(/[?#]/)[0] ?? ''
  const segs = noQuery.split('/').filter((s) => s !== '' && !s.startsWith('_'))
  return `/${segs.join('/')}`
}

export function componentForRoute(route: string): string {
  const seg = route.split('/')[1] ?? ''
  if (seg === 'nodes') return COMPONENT_OPTIONS.node
  if (seg === 'domains' || seg === 'dns') return COMPONENT_OPTIONS.ingress
  if (seg === 'builds') return COMPONENT_OPTIONS.build
  if (seg === 'secrets') return COMPONENT_OPTIONS.secrets
  if (seg === 'apps' || seg === 'deploys') return COMPONENT_OPTIONS.control
  return COMPONENT_OPTIONS.frontend
}

export function pickTemplate(ctx: {
  source: IssueSource
  pathname: string
}): IssueTemplate {
  if (ctx.source === 'feature') return 'feature_request'
  if (ctx.source === 'error') return 'bug_report'
  const p = ctx.pathname
  if (/^\/help(\/|$)/.test(p)) return 'docs_problem'
  if (/^\/(templates|catalog)(\/|$)/.test(p)) return 'template_request'
  if (/(^|\/)(import|migrat\w*)(\/|$)/.test(p)) return 'migration_problem'
  return 'bug_report'
}

export interface BrowserInfo {
  browser: string
  os: string
  arch: string
}

interface NavLike {
  userAgent?: string
  platform?: string
  userAgentData?: { platform?: string }
}

export function browserInfo(nav: NavLike): BrowserInfo {
  const ua = nav.userAgent ?? ''
  const m =
    /(Edg|OPR|Firefox|Chrome|Version)\/(\d+)/.exec(ua) ??
    /(Safari)\/(\d+)/.exec(ua)
  const names: Record<string, string> = {
    Edg: 'Edge',
    OPR: 'Opera',
    Version: 'Safari',
  }
  const browser = m ? `${names[m[1] ?? ''] ?? m[1]} ${m[2]}` : 'unknown'
  const plat = (nav.userAgentData?.platform ?? nav.platform ?? ua).toString()
  let os = 'unknown'
  if (/mac/i.test(plat)) os = 'macOS'
  else if (/win/i.test(plat)) os = 'Windows'
  else if (/android/i.test(ua)) os = 'Android'
  else if (/linux|x11/i.test(plat + ua)) os = 'Linux'
  const arch = /arm|aarch64/i.test(plat + ua)
    ? 'arm'
    : /x86_64|x64|win64|wow64/i.test(ua)
      ? 'x86_64'
      : ''
  return { browser, os, arch }
}

export interface IssueContext {
  source: IssueSource
  pathname: string
  /** Route template, e.g. /apps/$name. */
  route: string
  version?: string
  browser?: BrowserInfo
  errorMessage?: string
  requestId?: string
}

export function buildDraft(ctx: IssueContext): IssueDraft {
  const template = pickTemplate(ctx)
  const fields: Record<string, string> = {}
  const env = [
    ctx.version ? `Dashboard version: ${ctx.version}` : '',
    ctx.browser
      ? `Browser: ${ctx.browser.browser} on ${ctx.browser.os}${ctx.browser.arch ? ` (${ctx.browser.arch})` : ''}`
      : '',
    `Page: ${ctx.route}`,
  ]
    .filter(Boolean)
    .join('\n')
  if (template === 'bug_report') {
    if (ctx.version) fields.version = ctx.version
    fields.component = componentForRoute(ctx.route)
    fields.context = env
    if (ctx.errorMessage) {
      fields.actual = [
        redactText(ctx.errorMessage),
        ctx.requestId ? `Request ID: ${ctx.requestId}` : '',
      ]
        .filter(Boolean)
        .join('\n')
    } else fields.actual = ''
  } else if (template === 'docs_problem') {
    fields.page = ctx.pathname.replace(/^\/help\/?/, '') || 'help'
    fields.problem = ''
  } else if (template === 'migration_problem') {
    if (ctx.version) fields.version = ctx.version
    fields.failed = ''
  } else if (template === 'feature_request') {
    fields.problem = ''
    fields.context = env
  } else {
    fields.service = ''
  }
  return { template, title: TITLE_PREFIX[template], fields }
}

function enc(v: string): string {
  return encodeURIComponent(v)
}

// Cuts at a code point boundary so a surrogate pair is never split.
function cut(value: string, len: number): string {
  let end = Math.max(0, len)
  const c = value.charCodeAt(end - 1)
  if (c >= 0xd800 && c <= 0xdbff) end -= 1
  return value.slice(0, end)
}

function render(
  base: string,
  template: IssueTemplate,
  title: string,
  fields: Record<string, string>,
): string {
  const parts = [`template=${enc(`${template}.yml`)}`]
  if (title.trim()) parts.push(`title=${enc(title)}`)
  for (const [k, v] of Object.entries(fields)) {
    if (v !== '') parts.push(`${enc(k)}=${enc(v)}`)
  }
  return `${base}/new?${parts.join('&')}`
}

export function buildIssueUrl(
  draft: IssueDraft,
  supportUrl: string,
  maxChars: number,
): string {
  const trimmed = supportUrl.replace(/\/+$/, '')
  if (!trimmed.endsWith('/issues')) return supportUrl
  const fields = { ...draft.fields }
  const fits = () =>
    render(trimmed, draft.template, draft.title, fields).length <= maxChars

  for (const id of TRUNCATE_ORDER) {
    const full = fields[id]
    if (fits() || !full) continue
    let lo = 0
    let hi = full.length
    while (lo < hi) {
      const mid = Math.ceil((lo + hi) / 2)
      fields[id] = cut(full, mid) + ELLIPSIS
      if (fits()) lo = mid
      else hi = mid - 1
    }
    fields[id] = lo > 0 ? cut(full, lo) + ELLIPSIS : ''
  }
  for (const id of DROP_ORDER) {
    if (fits()) break
    delete fields[id]
  }
  return render(trimmed, draft.template, draft.title, fields)
}
