// Structured JSON field filters for log search. The wire form is one string
// per filter ("key=value", "key!=value", "key~substring"), repeated as
// `field` query params, matching GET /api/v1/apps/{name}/logs.

export type FieldOp = '=' | '!=' | '~'

export interface FieldFilter {
  key: string
  op: FieldOp
  value: string
}

export const FIELD_OPS: FieldOp[] = ['=', '!=', '~']

export function formatFieldFilter(f: FieldFilter): string {
  return `${f.key}${f.op}${f.value}`
}

// parseFieldFilter returns null for text that is not key, operator, value.
export function parseFieldFilter(raw: string): FieldFilter | null {
  const m = /^([A-Za-z0-9_.\-[\]]+)(!=|=|~)(.*)$/.exec(raw.trim())
  if (!m) {
    return null
  }
  const [, key, op, value] = m
  if (!key || !op || value === undefined || value === '') {
    return null
  }
  return { key, op: op as FieldOp, value }
}

export function parseFieldFilters(raw: readonly string[]): FieldFilter[] {
  const out: FieldFilter[] = []
  for (const r of raw) {
    const f = parseFieldFilter(r)
    if (f) {
      out.push(f)
    }
  }
  return out
}

// appendFieldParams adds one `field` param per filter.
export function appendFieldParams(
  params: URLSearchParams,
  filters: readonly FieldFilter[],
): void {
  for (const f of filters) {
    params.append('field', formatFieldFilter(f))
  }
}

export interface LogQueryState {
  q?: string
  level?: string
  container?: string
  stream?: string
  fields?: readonly FieldFilter[]
  from?: Date
  to?: Date
  limit?: number
}

// buildLogQuery is the single place the dashboard and permalinks turn a
// filter state into query params.
export function buildLogQuery(state: LogQueryState): URLSearchParams {
  const params = new URLSearchParams()
  if (state.from) {
    params.set('from', state.from.toISOString())
  }
  if (state.to) {
    params.set('to', state.to.toISOString())
  }
  if (state.q) {
    params.set('q', state.q)
  }
  if (state.level) {
    params.set('level', state.level)
  }
  if (state.container) {
    params.set('container', state.container)
  }
  if (state.stream) {
    params.set('stream', state.stream)
  }
  if (state.fields) {
    appendFieldParams(params, state.fields)
  }
  if (state.limit) {
    params.set('limit', String(state.limit))
  }
  return params
}
