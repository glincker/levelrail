import type {
  ConsoleQuery,
  ConsoleResource,
  ResourceKind,
} from '../queries/resources'
import { RESOURCE_KINDS } from '../queries/resources'

export interface ResourceTarget {
  to: string
  params: Record<string, string>
}

export function resourceTarget(r: ConsoleResource): ResourceTarget {
  switch (r.kind) {
    case 'database':
      return { to: '/databases/$name', params: { name: r.name } }
    case 'node':
      return { to: '/nodes/$id', params: { id: r.id ?? r.name } }
    case 'domain':
      return { to: '/domains/$domain', params: { domain: r.name } }
    case 'network':
      return { to: '/network/private', params: {} }
    case 'bucket':
      return { to: '/settings/storage', params: {} }
    default:
      return { to: '/apps/$name', params: { name: r.name } }
  }
}

export function resourceKey(r: Pick<ConsoleResource, 'kind' | 'name'>): string {
  return `${r.kind}:${r.name}`
}

export function isResourceKind(v: unknown): v is ResourceKind {
  return RESOURCE_KINDS.some((k) => k === v)
}

export function emptyQuery(): ConsoleQuery {
  return { q: '', kind: '', tags: [] }
}

export function serializeView(q: ConsoleQuery): string {
  return JSON.stringify({ q: q.q, kind: q.kind, tags: q.tags })
}

// A saved view is stored as opaque JSON, so anything unexpected degrades to an empty filter.
export function parseView(raw: string): ConsoleQuery {
  try {
    const v = JSON.parse(raw) as Record<string, unknown>
    return {
      q: typeof v.q === 'string' ? v.q : '',
      kind: isResourceKind(v.kind) ? v.kind : '',
      tags: Array.isArray(v.tags)
        ? v.tags.filter((t): t is string => typeof t === 'string')
        : [],
    }
  } catch {
    return emptyQuery()
  }
}
