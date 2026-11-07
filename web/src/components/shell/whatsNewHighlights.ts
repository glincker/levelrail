// Client-side picker that turns raw changelog bullets into a few
// user-visible highlight cards. Pure and deterministic: same bullets in,
// same cards out. The server data shape is untouched.

export type WhatsNewGroup = 'apps' | 'domains' | 'storage' | 'nextjs' | 'other'
export type WhatsNewRoute =
  '/domains' | '/storage' | '/apps' | '/deployments' | '/settings'

export interface ParsedBullet {
  index: number
  scope: string
  text: string
  title: string
  detail: string
  group: WhatsNewGroup
  route: WhatsNewRoute | null
  score: number
  internal: boolean
}

// Rule 1: bullets that read as developer plumbing never become a card.
const INTERNAL =
  /\b(hooks?|refactor(ed|ing)?|tests?|ci|lint|abilit(y|ies)|mocks?|fixtures?|e2e|storybook|typecheck|lockfile|chore|ratchet|coverage|bump(ed)?|deps?|internal)\b/i
const VERB = /^(add|adds|added|new|support|introduce|let|allow|show|shows)\b/i
const SURFACE =
  /\b(page|dashboard|button|header|banner|wizard|card|toast|panel|preview|plan|redirect|picker|form)\b/i

const GROUP_PATTERNS: [WhatsNewGroup, RegExp][] = [
  ['nextjs', /next\.?js/i],
  ['domains', /\b(domains?|dns|www|tls|certificates?)\b/i],
  ['storage', /\b(storage|volumes?|buckets?|backups?|s3)\b/i],
  ['apps', /\b(apps?|stacks?)\b/i],
]

const GROUP_ROUTE: Partial<Record<WhatsNewGroup, WhatsNewRoute>> = {
  domains: '/domains',
  storage: '/storage',
  apps: '/apps',
  nextjs: '/apps',
}

export const GROUP_ORDER: WhatsNewGroup[] = [
  'apps',
  'domains',
  'storage',
  'nextjs',
  'other',
]

const SCOPE_RE = /^\*{0,2}([A-Za-z][\w ./-]{1,23}):\*{0,2}\s+(.+)$/
// En and em dash code points, built numerically to keep them out of source.
const LONG_DASH = new RegExp(
  `\\s*[${String.fromCharCode(0x2013)}${String.fromCharCode(0x2014)}]\\s*`,
  'g',
)

// Strips markdown, PR refs and long dashes so no raw syntax reaches the UI.
export function stripMarkdown(raw: string): string {
  return raw
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/[*_`]/g, '')
    .replace(/\s*\((#\d+|[0-9a-f]{7,40})\)/g, '')
    .replace(LONG_DASH, ', ')
    .replace(/\s+/g, ' ')
    .trim()
}

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

function detectGroup(scope: string, text: string): WhatsNewGroup {
  for (const [group, re] of GROUP_PATTERNS) if (re.test(scope)) return group
  for (const [group, re] of GROUP_PATTERNS) if (re.test(text)) return group
  return 'other'
}

// Rule 2: a keyword to route map, never a guess. No match means no CTA.
function detectRoute(group: WhatsNewGroup, text: string): WhatsNewRoute | null {
  const direct = GROUP_ROUTE[group]
  if (direct) return direct
  if (/\b(deploys?|deployments?|rollouts?|builds?)\b/i.test(text)) {
    return '/deployments'
  }
  if (/\b(settings?|preferences?)\b/i.test(text)) return '/settings'
  return null
}

// Splits a sentence into a short title and a muted detail at the first
// break, only when the title half is a sensible length.
function splitTitle(text: string): { title: string; detail: string } {
  const m = /[,;:] | \(| for | so | to | with | when /.exec(text)
  if (m && m.index >= 12 && m.index <= 70) {
    const rest = text.slice(m.index + m[0].length).trim()
    if (rest.length > 0) {
      const lead = /^[,;:]/.test(m[0]) ? '' : `${m[0].trim()} `
      return {
        title: capitalize(text.slice(0, m.index).trim()),
        detail: capitalize(`${lead}${rest}`.replace(/\)$/, '')),
      }
    }
  }
  return { title: capitalize(text), detail: '' }
}

export function parseBullet(raw: string, index: number): ParsedBullet {
  const m = SCOPE_RE.exec(raw.trim())
  const scope = m ? stripMarkdown(m[1] ?? '').toLowerCase() : ''
  const text = stripMarkdown(m ? (m[2] ?? '') : raw)
  const group = detectGroup(scope, text)
  const route = detectRoute(group, text)
  const internal = INTERNAL.test(`${scope} ${text}`)
  // Rule 3: score = known surface + route + "new thing" verb + UI noun; long text loses.
  let score = 0
  if (group !== 'other') score += 3
  if (route) score += 1
  if (VERB.test(text)) score += 2
  if (SURFACE.test(text)) score += 1
  if (text.length > 140) score -= 1
  const { title, detail } = splitTitle(text)
  return {
    index,
    scope,
    text: capitalize(text),
    title,
    detail,
    group,
    route,
    score,
    internal,
  }
}

export function maxHighlights(): number {
  const n = Number(import.meta.env.VITE_WHATSNEW_MAX_HIGHLIGHTS ?? 3)
  return Number.isFinite(n) ? Math.min(4, Math.max(2, Math.floor(n))) : 3
}

export function minHighlightScore(): number {
  const n = Number(import.meta.env.VITE_WHATSNEW_MIN_SCORE ?? 3)
  return Number.isFinite(n) ? n : 3
}

export interface HighlightSplit {
  highlights: ParsedBullet[]
  rest: ParsedBullet[]
}

// Rule 4: top scorers (stable by position) become cards; fewer than the
// cap is fine, nothing is padded. Everything else goes to "All changes".
export function pickHighlights(
  bullets: string[],
  max: number = maxHighlights(),
): HighlightSplit {
  const parsed = bullets.map(parseBullet).filter((b) => b.text.length > 0)
  const min = minHighlightScore()
  const chosen = parsed
    .filter((b) => !b.internal && b.score >= min)
    .sort((a, b) => b.score - a.score || a.index - b.index)
    .slice(0, max)
    .sort((a, b) => a.index - b.index)
  const ids = new Set(chosen.map((b) => b.index))
  return { highlights: chosen, rest: parsed.filter((b) => !ids.has(b.index)) }
}

export function groupBullets(
  items: ParsedBullet[],
): { group: WhatsNewGroup; items: ParsedBullet[] }[] {
  return GROUP_ORDER.map((group) => ({
    group,
    items: items.filter((b) => b.group === group),
  })).filter((g) => g.items.length > 0)
}

function envInt(raw: unknown, fallback: number, min: number): number {
  const n = Number(raw ?? fallback)
  return Number.isFinite(n) ? Math.max(min, Math.floor(n)) : fallback
}

// A squash release can carry hundreds of bullets; above this the panel groups them.
export function maxListSize(): number {
  return envInt(import.meta.env.VITE_WHATSNEW_MAX_LIST, 40, 1)
}

export function maxScopeGroups(): number {
  return envInt(import.meta.env.VITE_WHATSNEW_MAX_GROUPS, 8, 1)
}

// Hard ceiling on rendered rows so a huge release never floods the DOM.
export const MAX_ROWS = 200

export interface ScopeGroup {
  scope: string
  items: ParsedBullet[]
}

// Groups by bullet scope (empty scope is one "other" bucket), biggest first, top N only.
export function scopeGroups(
  items: ParsedBullet[],
  top: number = maxScopeGroups(),
): ScopeGroup[] {
  const map = new Map<string, ParsedBullet[]>()
  for (const b of items) {
    const list = map.get(b.scope) ?? []
    list.push(b)
    map.set(b.scope, list)
  }
  return [...map.entries()]
    .map(([scope, list]) => ({ scope, items: list }))
    .sort((a, b) => b.items.length - a.items.length)
    .slice(0, top)
}
