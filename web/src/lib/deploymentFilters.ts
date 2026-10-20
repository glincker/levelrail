import type { Deployment } from '../types/deployment'

export interface DeploymentFilters {
  status: string[]
  trigger: string[]
  app: string
  environment: string
  branch: string
  author: string
  q: string
}

export interface DeploymentsSearch extends DeploymentFilters {
  d: string
}

export type FilterKey = Exclude<keyof DeploymentFilters, 'q'>

export const EMPTY_FILTERS: DeploymentFilters = {
  status: [],
  trigger: [],
  app: '',
  environment: '',
  branch: '',
  author: '',
  q: '',
}

function str(v: unknown): string {
  return typeof v === 'string' ? v : ''
}

function list(v: unknown): string[] {
  const raw = Array.isArray(v)
    ? v.map(String)
    : typeof v === 'string'
      ? [v]
      : []
  const out: string[] = []
  for (const part of raw.flatMap((r) => r.split(','))) {
    const t = part.trim()
    if (t && !out.includes(t)) out.push(t)
  }
  return out
}

export function parseDeploymentsSearch(
  search: Record<string, unknown>,
): DeploymentsSearch {
  return {
    status: list(search.status),
    trigger: list(search.trigger),
    app: str(search.app),
    environment: str(search.environment),
    branch: str(search.branch),
    author: str(search.author),
    q: str(search.q),
    d: str(search.d),
  }
}

export type UrlSearch = Partial<
  Record<keyof DeploymentsSearch, string | string[]>
>

/** Search params for the URL: empty values are omitted so the URL stays short. */
export function toUrlSearch(filters: DeploymentFilters, d = ''): UrlSearch {
  const out: UrlSearch = {}
  if (filters.status.length) out.status = filters.status
  if (filters.trigger.length) out.trigger = filters.trigger
  if (filters.app) out.app = filters.app
  if (filters.environment) out.environment = filters.environment
  if (filters.branch) out.branch = filters.branch
  if (filters.author) out.author = filters.author
  if (filters.q) out.q = filters.q
  if (d) out.d = d
  return out
}

/** Query string for GET /api/v1/deployments. Author is applied client side. */
export function toApiParams(
  filters: DeploymentFilters,
  cursor = '',
  limit = 50,
): URLSearchParams {
  const p = new URLSearchParams()
  if (filters.status.length) p.set('status', filters.status.join(','))
  if (filters.trigger.length) p.set('trigger', filters.trigger.join(','))
  if (filters.app) p.set('app', filters.app)
  if (filters.environment) p.set('environment', filters.environment)
  if (filters.branch) p.set('branch', filters.branch)
  if (filters.q) p.set('q', filters.q)
  if (cursor) p.set('cursor', cursor)
  p.set('limit', String(limit))
  return p
}

export function serverFilters(f: DeploymentFilters): DeploymentFilters {
  return { ...f, author: '' }
}

export function activeFilterCount(f: DeploymentFilters): number {
  return (
    f.status.length +
    f.trigger.length +
    [f.app, f.environment, f.branch, f.author, f.q].filter(Boolean).length
  )
}

export function matchesFilters(d: Deployment, f: DeploymentFilters): boolean {
  if (f.status.length && !f.status.includes(d.status)) return false
  if (f.trigger.length && !f.trigger.includes(d.trigger)) return false
  if (f.app && d.app !== f.app) return false
  if (
    f.environment &&
    d.environment.toLowerCase() !== f.environment.toLowerCase()
  ) {
    return false
  }
  if (f.branch && d.branch !== f.branch) return false
  if (f.author && d.author !== f.author) return false
  if (f.q) {
    const q = f.q.toLowerCase()
    const hit =
      d.commit_message.toLowerCase().includes(q) ||
      d.commit_sha.toLowerCase().startsWith(q) ||
      d.app.toLowerCase().includes(q)
    if (!hit) return false
  }
  return true
}

export interface Facets {
  app: string[]
  environment: string[]
  branch: string[]
  author: string[]
}

/** Option lists for the filter menu, drawn from rows already loaded. */
export function facetsFrom(rows: Deployment[]): Facets {
  const uniq = (pick: (d: Deployment) => string) =>
    [...new Set(rows.map(pick).filter(Boolean))].sort((a, b) =>
      a.localeCompare(b),
    )
  return {
    app: uniq((d) => d.app),
    environment: uniq((d) => d.environment),
    branch: uniq((d) => d.branch),
    author: uniq((d) => d.author),
  }
}

export function removeFilter(
  f: DeploymentFilters,
  key: keyof DeploymentFilters,
  value?: string,
): DeploymentFilters {
  if (key === 'status' || key === 'trigger') {
    return { ...f, [key]: value ? f[key].filter((v) => v !== value) : [] }
  }
  return { ...f, [key]: '' }
}
