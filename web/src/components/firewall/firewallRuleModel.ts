import type {
  FwAction,
  FwCondition,
  FwField,
  FwFieldError,
  FwOp,
  FwRule,
  FwRuleInput,
} from '../../queries/firewallEdge'
import type { RuleDraft, RuleErrors } from '../rules/ruleModel'

export const FW_FIELD_OPS: Record<FwField, FwOp[]> = {
  path: ['equals', 'starts_with', 'contains', 'glob'],
  host: ['equals', 'contains', 'glob'],
  method: ['equals'],
  header: ['equals', 'starts_with', 'contains', 'glob', 'exists'],
  query: ['equals', 'starts_with', 'contains', 'exists'],
  user_agent: ['equals', 'starts_with', 'contains', 'glob'],
  ip: ['in_cidr'],
  country: ['equals'],
}

export const FW_ACTIONS: FwAction[] = [
  'allow',
  'deny',
  'challenge',
  'log',
  'rate_limit',
]

const METHODS = [
  'GET',
  'HEAD',
  'POST',
  'PUT',
  'PATCH',
  'DELETE',
  'OPTIONS',
  'CONNECT',
  'TRACE',
]
const MAX_NAME = 80
const MAX_NOTE = 200
const MAX_VALUE = 256
const MAX_PARAM_NAME = 64
const MAX_WILDCARDS = 8
const MAX_RATE = 1_000_000
const MAX_WINDOW = 3600

/** Error messages the validator can raise, supplied translated by the caller. */
export interface FwMessages {
  nameRequired: string
  noteTooLong: string
  actionRequired: string
  rateCount: string
  rateWindow: string
  paramName: string
  valueRequired: string
  ipInvalid: string
  methodInvalid: string
  countryInvalid: string
  pathSlash: string
  wildcards: string
}

const IPV4 = /^(\d{1,3})(\.\d{1,3}){3}(\/\d{1,2})?$/
const IPV6 = /^[0-9a-fA-F:]+(\/\d{1,3})?$/

export function isIpOrCidr(v: string): boolean {
  if (IPV4.test(v)) {
    const [addr, bits] = v.split('/')
    const ok = (addr ?? '').split('.').every((p) => Number(p) <= 255)
    return ok && (bits === undefined || Number(bits) <= 32)
  }
  if (v.includes(':') && IPV6.test(v)) {
    const bits = v.split('/')[1]
    return bits === undefined || Number(bits) <= 128
  }
  return false
}

function checkCondition(
  c: FwCondition,
  at: string,
  m: FwMessages,
  out: RuleErrors,
) {
  const named = c.field === 'header' || c.field === 'query'
  if (
    named &&
    (!c.name ||
      c.name.length > MAX_PARAM_NAME ||
      !/^[A-Za-z0-9_-]+$/.test(c.name))
  ) {
    out[`${at}.name`] = m.paramName
  }
  if (c.op === 'exists') return
  const v = c.value ?? ''
  if (v === '' || v.length > MAX_VALUE) {
    out[`${at}.value`] = m.valueRequired
  } else if (c.field === 'ip') {
    if (!isIpOrCidr(v)) out[`${at}.value`] = m.ipInvalid
  } else if (c.field === 'method') {
    if (!METHODS.includes(v.toUpperCase())) out[`${at}.value`] = m.methodInvalid
  } else if (c.field === 'country') {
    if (!/^[A-Za-z]{2}$/.test(v)) out[`${at}.value`] = m.countryInvalid
  } else if (
    c.field === 'path' &&
    (c.op === 'equals' || c.op === 'starts_with') &&
    !v.startsWith('/')
  ) {
    out[`${at}.value`] = m.pathSlash
  } else if (
    c.op === 'glob' &&
    (v.match(/[*?]/g)?.length ?? 0) > MAX_WILDCARDS
  ) {
    out[`${at}.value`] = m.wildcards
  }
}

/** Mirrors internal/edgefw.ValidateRule so most mistakes show before a request is made. */
export function validateDraft(d: RuleDraft, m: FwMessages): RuleErrors {
  const out: RuleErrors = {}
  const name = d.name.trim()
  if (name === '' || name.length > MAX_NAME) out.name = m.nameRequired
  if (d.note.trim().length > MAX_NOTE) out.note = m.noteTooLong
  if (!FW_ACTIONS.includes(d.action as FwAction)) out.action = m.actionRequired
  if (d.action === 'rate_limit') {
    const n = Number(d.params.rate_count)
    const w = Number(d.params.rate_window_seconds)
    if (!Number.isInteger(n) || n < 1 || n > MAX_RATE)
      out['params.rate_count'] = m.rateCount
    if (!Number.isInteger(w) || w < 1 || w > MAX_WINDOW)
      out['params.rate_window_seconds'] = m.rateWindow
  }
  d.conditions.forEach((c, i) =>
    checkCondition(
      {
        field: c.field as FwField,
        op: c.op as FwOp,
        name: c.name.trim(),
        value: c.value.trim(),
      },
      `conditions[${i}]`,
      m,
      out,
    ),
  )
  return out
}

/** Maps the server's field names onto the builder's error keys. */
export function mapServerErrors(fields: FwFieldError[]): RuleErrors {
  const out: RuleErrors = {}
  for (const f of fields) {
    const key =
      f.field === 'rate_count' || f.field === 'rate_window_seconds'
        ? `params.${f.field}`
        : f.field
    if (!(key in out)) out[key] = f.message
  }
  return out
}

export function draftFromRule(r: FwRuleInput): RuleDraft {
  return {
    name: r.name,
    note: r.note ?? '',
    conditions: r.conditions.map((c) => ({
      field: c.field,
      op: c.op,
      name: c.name ?? '',
      value: c.value ?? '',
    })),
    action: r.action,
    params:
      r.action === 'rate_limit'
        ? {
            rate_count: String(r.rate_count ?? ''),
            rate_window_seconds: String(r.rate_window_seconds ?? ''),
          }
        : {},
  }
}

export function ruleFromDraft(
  d: RuleDraft,
  scope: string,
  enabled: boolean,
): FwRuleInput {
  const rule: FwRuleInput = {
    name: d.name.trim(),
    enabled,
    action: d.action as FwAction,
    conditions: d.conditions.map((c) => {
      const field = c.field as FwField
      const out: FwCondition = { field, op: c.op as FwOp }
      if (field === 'header' || field === 'query') out.name = c.name.trim()
      if (c.op !== 'exists') out.value = c.value.trim()
      return out
    }),
  }
  if (d.note.trim()) rule.note = d.note.trim()
  if (scope) rule.scope = scope
  if (d.action === 'rate_limit') {
    rule.rate_count = Number(d.params.rate_count)
    rule.rate_window_seconds = Number(d.params.rate_window_seconds)
  }
  return rule
}

export function ruleJson(
  d: RuleDraft,
  scope: string,
  enabled: boolean,
): string {
  return JSON.stringify(ruleFromDraft(d, scope, enabled), null, 2)
}

export function conditionText(c: FwCondition): string {
  const field = c.name ? `${c.field}:${c.name}` : c.field
  return [field, c.op.replace('_', ' '), c.value].filter(Boolean).join(' ')
}

export function isIpListRule(r: FwRule): FwAction | null {
  const only = r.conditions.length === 1 ? r.conditions[0] : undefined
  if (only?.field === 'ip' && (r.action === 'deny' || r.action === 'allow')) {
    return r.action
  }
  return null
}

/** A starting draft for "Create rule from this" on a traffic row. */
export function draftForValue(
  field: FwField,
  value: string,
  name = '',
): RuleDraft {
  const op: FwOp =
    field === 'ip'
      ? 'in_cidr'
      : field === 'path'
        ? 'starts_with'
        : field === 'host'
          ? 'equals'
          : 'contains'
  return {
    name: '',
    note: '',
    conditions: [{ field, op, name, value }],
    action: '',
    params: {},
  }
}

/** The editable part of a stored rule, ready to send back to the server. */
export function toRuleInput(r: FwRule): FwRuleInput {
  return {
    name: r.name,
    note: r.note,
    scope: r.scope,
    enabled: r.enabled,
    conditions: r.conditions,
    action: r.action,
    rate_count: r.rate_count,
    rate_window_seconds: r.rate_window_seconds,
  }
}

/** The id order that results from moving rule `id` one step among `custom` inside the full list. */
export function movedOrder(
  all: FwRule[],
  custom: FwRule[],
  id: string,
  dir: -1 | 1,
): string[] | null {
  const at = custom.findIndex((r) => r.id === id)
  const other = custom[at + dir]
  if (at < 0 || !other) return null
  const ids = all.map((r) => r.id)
  const i = ids.indexOf(id)
  const j = ids.indexOf(other.id)
  ids[i] = other.id
  ids[j] = id
  return ids
}
