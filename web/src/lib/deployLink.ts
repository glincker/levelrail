/** Parsing and validation for "Deploy with" links: /apps/new?deploy=1&image=... */

const COMPONENT = '[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*'
const DOMAIN_LABEL = '[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?'
const HOST = `${DOMAIN_LABEL}(?:\\.${DOMAIN_LABEL})*(?::[0-9]+)?`
const TAG = '[A-Za-z0-9_][A-Za-z0-9._-]{0,127}'
const IMAGE_RE = new RegExp(
  `^(?:${HOST}/)?${COMPONENT}(?:/${COMPONENT})*(?::${TAG})?(?:@sha256:[a-f0-9]{64})?$`,
)
const DNS_LABEL_RE = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/
const ENV_KEY_RE = /^[A-Z_][A-Z0-9_]{0,63}$/
const MAX_ENV_KEYS = 50
const MAX_UNKNOWN_REPORTED = 5

/** Params the link scheme defines; everything else is reported as ignored. */
export const DEPLOY_LINK_PARAMS = [
  'deploy',
  'image',
  'name',
  'port',
  'env',
  'health',
  'source',
] as const

export interface DeployLinkPrefill {
  /** True when the URL carried deploy=1, even if every other param was invalid. */
  active: boolean
  image?: string
  name?: string
  port?: number
  /** Env var names only; values are never read from a link. */
  env: string[]
  health?: string
  sourceUrl?: string
  sourceHost?: string
  /** Param names that were present but rejected, for the banner. */
  ignored: string[]
}

export function isValidImageRef(v: string): boolean {
  return v.length > 0 && v.length <= 255 && IMAGE_RE.test(v)
}

export function isValidAppName(v: string): boolean {
  return DNS_LABEL_RE.test(v)
}

export function isValidPort(v: string): boolean {
  if (!/^[0-9]{1,5}$/.test(v)) return false
  const n = Number(v)
  return n >= 1 && n <= 65535
}

export function isValidEnvKey(v: string): boolean {
  return ENV_KEY_RE.test(v)
}

export function isValidHealthPath(v: string): boolean {
  if (v.length > 256 || !v.startsWith('/') || v.startsWith('//')) return false
  for (const ch of v) {
    const c = ch.codePointAt(0) ?? 0
    if (c <= 0x20 || c === 0x7f || ch === '\\') return false
  }
  return true
}

/** Returns the host for an http(s) URL without userinfo, else null. */
export function sourceHostOf(v: string): string | null {
  if (v.length > 2048) return null
  let u: URL
  try {
    u = new URL(v)
  } catch {
    return null
  }
  if (u.protocol !== 'https:' && u.protocol !== 'http:') return null
  if (u.username !== '' || u.password !== '' || u.hostname === '') return null
  return u.host
}

function slugFromImage(image: string): string {
  const last = image.split('@')[0]?.split('/').pop() ?? ''
  const slug = last
    .replace(/:[^:]*$/, '')
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 63)
    .replace(/-+$/g, '')
  return isValidAppName(slug) ? slug : ''
}

/**
 * parseDeployLink validates a link's query params. Invalid params are dropped
 * and named in `ignored`; nothing here ever throws.
 */
export function parseDeployLink(
  search: Record<string, string>,
): DeployLinkPrefill {
  const out: DeployLinkPrefill = {
    active: search.deploy === '1',
    env: [],
    ignored: [],
  }
  if (!out.active) return out

  const known = new Set<string>(DEPLOY_LINK_PARAMS)
  const reject = (p: string) => out.ignored.push(p)

  const image = search.image?.trim()
  if (image !== undefined && image !== '') {
    if (isValidImageRef(image)) out.image = image
    else reject('image')
  }

  const name = search.name?.trim()
  if (name !== undefined && name !== '') {
    if (isValidAppName(name)) out.name = name
    else reject('name')
  }
  if (out.name === undefined && out.image !== undefined) {
    const slug = slugFromImage(out.image)
    if (slug !== '') out.name = slug
  }

  const port = search.port?.trim()
  if (port !== undefined && port !== '') {
    if (isValidPort(port)) out.port = Number(port)
    else reject('port')
  }

  const env = search.env
  if (env !== undefined && env.trim() !== '') {
    const seen = new Set<string>()
    let bad = false
    for (const raw of env.split(',')) {
      const key = raw.trim()
      if (key === '') continue
      if (!isValidEnvKey(key) || seen.size >= MAX_ENV_KEYS) {
        bad = true
        continue
      }
      seen.add(key)
    }
    out.env = [...seen]
    if (bad) reject('env')
  }

  const health = search.health?.trim()
  if (health !== undefined && health !== '') {
    if (isValidHealthPath(health)) out.health = health
    else reject('health')
  }

  const source = search.source?.trim()
  if (source !== undefined && source !== '') {
    const host = sourceHostOf(source)
    if (host !== null) {
      out.sourceUrl = source
      out.sourceHost = host
    } else reject('source')
  }

  const unknown = Object.keys(search).filter((k) => !known.has(k))
  for (const k of unknown.slice(0, MAX_UNKNOWN_REPORTED)) {
    reject(k.slice(0, 40))
  }
  return out
}

/** Coerces router search values (the router JSON-parses numbers) to strings. */
export function stringifySearch(
  search: Record<string, unknown>,
): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(search).slice(0, 40)) {
    if (typeof v === 'string') out[k] = v
    else if (typeof v === 'number' || typeof v === 'boolean') out[k] = String(v)
  }
  return out
}

/** Public page that forwards a deploy link to the visitor's own server. */
export const DEPLOY_BUTTON_PAGE = 'https://levelrail.com/deploy'
/** Badge served by the jsDelivr CDN from this repository, so README views do not reach the site. */
export const DEPLOY_BUTTON_BADGE =
  'https://cdn.jsdelivr.net/gh/glincker/levelrail@main/docs/assets/brand/deploy-button-split.svg'

export interface DeployButtonInput {
  image: string
  name?: string
  port?: string
  /** Comma or newline separated env var names; values are never part of a link. */
  env?: string
  health?: string
  source?: string
}

export interface DeployButtonResult {
  url: string | null
  /** Field names that were present but invalid, so the form can flag them. */
  invalid: string[]
}

/** Builds a README button URL from form fields using the same rules as parseDeployLink. */
export function buildDeployButtonUrl(
  input: DeployButtonInput,
): DeployButtonResult {
  const invalid: string[] = []
  const q = new URLSearchParams()
  const image = input.image.trim()
  if (!isValidImageRef(image)) invalid.push('image')
  else q.set('image', image)

  const check = (
    key: 'name' | 'port' | 'health' | 'source',
    ok: (v: string) => boolean,
  ) => {
    const v = (input[key] ?? '').trim()
    if (v === '') return
    if (ok(v)) q.set(key, v)
    else invalid.push(key)
  }
  check('name', isValidAppName)
  check('port', isValidPort)
  check('health', isValidHealthPath)
  check('source', (v) => sourceHostOf(v) !== null)

  const keys = (input.env ?? '')
    .split(/[,\n]/)
    .map((k) => k.trim())
    .filter((k) => k !== '')
  if (keys.length > 0) {
    const unique = [...new Set(keys)]
    if (unique.length > MAX_ENV_KEYS || !unique.every(isValidEnvKey)) {
      invalid.push('env')
    } else q.set('env', unique.join(','))
  }
  if (invalid.length > 0) return { url: null, invalid }
  return { url: `${DEPLOY_BUTTON_PAGE}?${q.toString()}`, invalid }
}

/** Markdown snippet for a README. */
export function deployButtonMarkdown(url: string): string {
  return `[![Deploy to Levelrail](${DEPLOY_BUTTON_BADGE})](${url})`
}
