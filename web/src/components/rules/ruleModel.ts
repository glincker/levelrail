export interface OperatorDef {
  id: string
  label: string
  /** The operator takes no value, for example "exists". */
  valueless?: boolean
}

export interface FieldDef {
  id: string
  label: string
  operators: OperatorDef[]
  /** The field is addressed by a name, such as a header or query parameter. */
  named?: boolean
  namePlaceholder?: string
  valuePlaceholder?: string
}

export interface ParamDef {
  id: string
  label: string
  kind: 'text' | 'number' | 'select'
  options?: { value: string; label: string }[]
  placeholder?: string
  min?: number
  max?: number
  unit?: string
}

/** What a rule does when its conditions match; the caller owns the catalog. */
export interface ActionDef {
  id: string
  label: string
  hint?: string
  params?: ParamDef[]
}

export interface RuleCondition {
  field: string
  op: string
  name: string
  value: string
}

export interface RuleDraft {
  name: string
  note: string
  conditions: RuleCondition[]
  action: string
  params: Record<string, string>
}

/** Error text keyed by a path such as `name`, `conditions[0].value` or `params.rate_count`. */
export type RuleErrors = Record<string, string>

export function blankCondition(fields: FieldDef[]): RuleCondition {
  const f = fields[0]
  return {
    field: f?.id ?? '',
    op: f?.operators[0]?.id ?? '',
    name: '',
    value: '',
  }
}

export function blankDraft(fields: FieldDef[]): RuleDraft {
  return {
    name: '',
    note: '',
    conditions: [blankCondition(fields)],
    action: '',
    params: {},
  }
}

export function fieldOf(fields: FieldDef[], id: string): FieldDef | undefined {
  return fields.find((f) => f.id === id)
}

export function operatorOf(
  fields: FieldDef[],
  field: string,
  op: string,
): OperatorDef | undefined {
  return fieldOf(fields, field)?.operators.find((o) => o.id === op)
}

/** Changing the field keeps the operator when the new field supports it, else takes its first. */
export function changeField(
  fields: FieldDef[],
  c: RuleCondition,
  field: string,
): RuleCondition {
  const next = fieldOf(fields, field)
  const op = next?.operators.some((o) => o.id === c.op)
    ? c.op
    : (next?.operators[0]?.id ?? '')
  return { ...c, field, op, name: next?.named ? c.name : '' }
}

export function changeOperator(
  fields: FieldDef[],
  c: RuleCondition,
  op: string,
): RuleCondition {
  const valueless = operatorOf(fields, c.field, op)?.valueless
  return { ...c, op, value: valueless ? '' : c.value }
}

export function replaceAt<T>(list: T[], index: number, item: T): T[] {
  return list.map((x, i) => (i === index ? item : x))
}

export function removeAt<T>(list: T[], index: number): T[] {
  return list.filter((_, i) => i !== index)
}
