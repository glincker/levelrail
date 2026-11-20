import type { FieldValue, FormValues, KvEntry } from './documentYaml'
import {
  isSecretLikeName,
  type IacField,
  type IacKindModel,
} from './schemaToFields'

function envInt(raw: unknown, fallback: number): number {
  const n = Number(raw)
  return Number.isFinite(n) && n > 0 ? Math.floor(n) : fallback
}

// Schema patterns and typed input are untrusted, so oversize ones skip the regex to avoid a stalled tab.
const MAX_PATTERN_CHARS = envInt(
  import.meta.env.VITE_IAC_PATTERN_MAX_CHARS,
  256,
)
const MAX_PATTERN_INPUT_CHARS = envInt(
  import.meta.env.VITE_IAC_PATTERN_INPUT_MAX_CHARS,
  2048,
)

export interface FieldIssue {
  code:
    | 'required'
    | 'pattern'
    | 'min'
    | 'max'
    | 'minLength'
    | 'integer'
    | 'secretLiteral'
  param?: string | number
}

function hasValue(v: FieldValue | undefined): boolean {
  if (typeof v === 'string') return v.trim() !== ''
  if (Array.isArray(v)) return v.length > 0
  return v !== undefined
}

export function validateField(
  f: IacField,
  v: FieldValue | undefined,
): FieldIssue | null {
  if (f.type === 'keyValue') {
    const bad = (Array.isArray(v) ? (v as KvEntry[]) : []).find(
      (e) => !e.secret && e.value.trim() !== '' && isSecretLikeName(e.key),
    )
    if (bad) return { code: 'secretLiteral', param: bad.key }
  }
  if (!hasValue(v)) {
    return f.effectiveRequired && f.type !== 'boolean'
      ? { code: 'required' }
      : null
  }
  if (typeof v !== 'string') return null
  const s = v.trim()
  if (f.type === 'number') {
    const n = Number(s)
    if (!Number.isFinite(n) || (f.integer && !Number.isInteger(n)))
      return { code: 'integer' }
    if (f.min !== undefined && n < f.min) return { code: 'min', param: f.min }
    if (f.max !== undefined && n > f.max) return { code: 'max', param: f.max }
    return null
  }
  if (f.minLength !== undefined && s.length < f.minLength) {
    return { code: 'minLength', param: f.minLength }
  }
  if (
    f.pattern &&
    !f.secret &&
    f.pattern.length <= MAX_PATTERN_CHARS &&
    s.length <= MAX_PATTERN_INPUT_CHARS
  ) {
    try {
      if (!new RegExp(f.pattern).test(s)) return { code: 'pattern' }
    } catch {
      return null
    }
  }
  return null
}

export function validateValues(
  model: IacKindModel,
  values: FormValues,
): Record<string, FieldIssue> {
  const out: Record<string, FieldIssue> = {}
  for (const f of model.fields) {
    if (f.type === 'group') continue
    const issue = validateField(f, values[f.id])
    if (issue) out[f.id] = issue
  }
  return out
}
