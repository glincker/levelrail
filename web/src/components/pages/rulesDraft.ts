import type { UiRules } from '../../queries/pagesSites'

export const REDIRECT_STATUSES = [
  '301',
  '302',
  '303',
  '307',
  '308',
  '200',
  '404',
  '410',
  '451',
] as const

export interface DraftRedirect {
  id: string
  from: string
  to: string
  status: string
  force: boolean
}

export interface DraftHeader {
  id: string
  path: string
  setText: string
  removeText: string
}

export interface Draft {
  redirects: DraftRedirect[]
  headers: DraftHeader[]
}

let seq = 0
const nextId = () => `row-${++seq}`

export function newRedirect(from = ''): DraftRedirect {
  return { id: nextId(), from, to: '', status: '301', force: false }
}

export function newHeader(): DraftHeader {
  return { id: nextId(), path: '/*', setText: '', removeText: '' }
}

export function formatSet(set: Record<string, string> | undefined): string {
  return Object.entries(set ?? {})
    .map(([k, v]) => `${k}: ${v}`)
    .join('\n')
}

/** One "Name: value" per line; lines without a name are dropped. */
export function parseSet(text: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const i = line.indexOf(':')
    if (i < 0) continue
    const name = line.slice(0, i).trim()
    if (name) out[name] = line.slice(i + 1).trim()
  }
  return out
}

export function parseNames(text: string): string[] {
  return text
    .split(/[,\n]/)
    .map((n) => n.trim())
    .filter(Boolean)
}

export function draftFromUi(ui: UiRules): Draft {
  return {
    redirects: (ui.redirects ?? []).map((r) => ({
      id: nextId(),
      from: r.from,
      to: r.to,
      status: String(r.status ?? 301),
      force: Boolean(r.force),
    })),
    headers: (ui.headers ?? []).map((h) => ({
      id: nextId(),
      path: h.path,
      setText: formatSet(h.set),
      removeText: (h.remove ?? []).join(', '),
    })),
  }
}

/** The PUT body; also the canonical form used to tell whether the draft is dirty. */
export function draftToUi(d: Draft): UiRules {
  return {
    redirects: d.redirects.map((r) => ({
      from: r.from.trim(),
      to: r.to.trim(),
      status: Number(r.status),
      ...(r.force ? { force: true } : {}),
    })),
    headers: d.headers.map((h) => {
      const set = parseSet(h.setText)
      const remove = parseNames(h.removeText)
      return {
        path: h.path.trim(),
        ...(Object.keys(set).length > 0 ? { set } : {}),
        ...(remove.length > 0 ? { remove } : {}),
      }
    }),
  }
}

export function sameRules(a: UiRules, b: UiRules): boolean {
  return JSON.stringify(a) === JSON.stringify(b)
}

export function moveItem<T>(list: readonly T[], from: number, to: number): T[] {
  if (to < 0 || to >= list.length) return [...list]
  const next = [...list]
  const [item] = next.splice(from, 1)
  if (item !== undefined) next.splice(to, 0, item)
  return next
}
