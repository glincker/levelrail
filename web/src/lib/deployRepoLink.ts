/**
 * Parsing and building for git "Deploy to Levelrail" links: /deploy?repo=...
 * Mirrors internal/deploylink; both run internal/deploylink/testdata/cases.json.
 */
import { isValidAppName, isValidPort } from './deployLink'

const MAX_REPO_LEN = 2048
const MAX_BRANCH_LEN = 200
const HOST_RE =
  /^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?)*$/
const SSH_USER_RE = /^[a-z_][a-z0-9_-]{0,31}$/
const REPO_PATH_RE = /^[A-Za-z0-9._~/-]+$/
const SCP_RE = /^([a-z_][a-z0-9_-]{0,31})@([A-Za-z0-9.-]+):([A-Za-z0-9._~/-]+)$/
const BRANCH_RE = /^[A-Za-z0-9][A-Za-z0-9._/-]*$/
const TEMPLATE_RE = /^[a-z0-9][a-z0-9-]{0,62}$/
const MAX_UNKNOWN_REPORTED = 5

/** The only params a git link may carry. Env values and secrets are never read. */
export const DEPLOY_REPO_PARAMS = [
  'repo',
  'branch',
  'name',
  'template',
  'port',
] as const

function validRepoPath(p: string): boolean {
  return (
    p !== '' && REPO_PATH_RE.test(p) && !p.split('/').some((s) => s === '..')
  )
}

/** https or ssh git URL without credentials. */
export function isValidRepoUrl(v: string): boolean {
  if (v === '' || v.length > MAX_REPO_LEN) return false
  for (const ch of v) {
    const c = ch.codePointAt(0) ?? 0
    if (c <= 0x20 || c === 0x7f || ch === '\\') return false
  }
  if (!v.includes('://')) {
    const m = SCP_RE.exec(v)
    return m !== null && HOST_RE.test(m[2] ?? '') && validRepoPath(m[3] ?? '')
  }
  if (/[?#]/.test(v)) return false
  let u: URL
  try {
    u = new URL(v)
  } catch {
    return false
  }
  if (!HOST_RE.test(u.hostname)) return false
  if (u.port !== '' && !isValidPort(u.port)) return false
  if (u.protocol === 'https:') {
    if (u.username !== '' || u.password !== '') return false
  } else if (u.protocol === 'ssh:') {
    if (u.password !== '') return false
    if (u.username !== '' && !SSH_USER_RE.test(u.username)) return false
  } else return false
  // The raw text, since URL() would fold dot segments and hide a "..".
  const rest = v.slice(v.indexOf('://') + 3)
  const slash = rest.indexOf('/')
  return validRepoPath(slash < 0 ? '' : rest.slice(slash + 1))
}

export function isValidBranch(v: string): boolean {
  return (
    v !== '' &&
    v.length <= MAX_BRANCH_LEN &&
    BRANCH_RE.test(v) &&
    !v.includes('..') &&
    !v.includes('//') &&
    !v.endsWith('/') &&
    !v.endsWith('.lock') &&
    !v.endsWith('.')
  )
}

export function isValidTemplateId(v: string): boolean {
  return TEMPLATE_RE.test(v)
}

export interface RepoLinkPrefill {
  /** True when any link param was present, so the page shows the banner. */
  hasParams: boolean
  repo?: string
  branch?: string
  name?: string
  template?: string
  port?: number
  /** Param names that were present but rejected, for the warning banner. */
  ignored: string[]
}

/**
 * Validates /deploy query params. Invalid or unknown params are dropped and
 * named in `ignored`. `knownTemplates` is the catalog's ids once loaded;
 * undefined skips the existence check. Never throws.
 */
export function parseRepoLink(
  search: Record<string, string>,
  knownTemplates?: ReadonlySet<string>,
): RepoLinkPrefill {
  const out: RepoLinkPrefill = { hasParams: false, ignored: [] }
  const reject = (p: string) => out.ignored.push(p)
  const get = (k: string) => {
    const v = search[k]?.trim()
    return v === undefined || v === '' ? undefined : v
  }

  const repo = get('repo')
  if (repo !== undefined) {
    if (isValidRepoUrl(repo)) out.repo = repo
    else reject('repo')
  }
  const template = get('template')
  if (template !== undefined) {
    if (
      isValidTemplateId(template) &&
      (knownTemplates === undefined || knownTemplates.has(template))
    ) {
      out.template = template
    } else reject('template')
  }
  if (out.repo !== undefined && out.template !== undefined) {
    delete out.template
    reject('template')
  }
  const branch = get('branch')
  if (branch !== undefined) {
    if (out.repo !== undefined && isValidBranch(branch)) out.branch = branch
    else reject('branch')
  }
  const name = get('name')
  if (name !== undefined) {
    if (isValidAppName(name)) out.name = name
    else reject('name')
  }
  const port = get('port')
  if (port !== undefined) {
    if (isValidPort(port)) out.port = Number(port)
    else reject('port')
  }

  const known = new Set<string>(DEPLOY_REPO_PARAMS)
  for (const k of Object.keys(search)
    .filter((k) => !known.has(k))
    .slice(0, MAX_UNKNOWN_REPORTED)) {
    reject(k.slice(0, 40))
  }
  out.hasParams = Object.keys(search).length > 0
  return out
}

export interface RepoLinkInput {
  repo?: string
  branch?: string
  name?: string
  template?: string
  port?: string
}

export interface RepoLinkResult {
  url: string | null
  /** Field names that were present but invalid, so a form can flag them. */
  invalid: string[]
}

/** Builds the link for `origin` (a dashboard origin) using the same rules as parseRepoLink. */
export function buildRepoLinkUrl(
  origin: string,
  input: RepoLinkInput,
): RepoLinkResult {
  const invalid: string[] = []
  const q = new URLSearchParams()
  const field = (
    key: keyof RepoLinkInput,
    ok: (v: string) => boolean,
    required = false,
  ) => {
    const v = (input[key] ?? '').trim()
    if (v === '') {
      if (required) invalid.push(key)
      return
    }
    if (ok(v)) q.set(key, v)
    else invalid.push(key)
  }
  field('repo', isValidRepoUrl, true)
  field('branch', isValidBranch)
  field('name', isValidAppName)
  field('port', isValidPort)
  if (invalid.length > 0) return { url: null, invalid }
  return {
    url: `${origin.replace(/\/+$/, '')}/deploy?${q.toString()}`,
    invalid,
  }
}

/** Static badge in the public repository, served from its default branch. */
export const DEPLOY_BADGE_URL =
  'https://raw.githubusercontent.com/glincker/levelrail/main/docs/assets/deploy-to-levelrail.svg'

export function repoLinkMarkdown(alt: string, link: string): string {
  return `[![${alt}](${DEPLOY_BADGE_URL})](${link})`
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

export function repoLinkHtml(alt: string, link: string): string {
  return `<a href="${escapeHtml(link)}"><img src="${escapeHtml(DEPLOY_BADGE_URL)}" alt="${escapeHtml(alt)}" height="40"></a>`
}
