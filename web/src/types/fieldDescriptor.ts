// Wire shape of GET /api/v1/field-descriptors/{resource} (internal/fielddesc).
export type FieldType = 'string' | 'text' | 'secret' | 'select' | 'bool'

export interface FieldCondition {
  field: string
  equals: string[]
}

export interface FieldDescriptor {
  id: string
  key: string
  type: FieldType
  label_key: string
  help_key: string
  default?: string
  options?: string[]
  required: boolean
  secret: boolean
  show_when?: FieldCondition[]
  flag?: string
  form_only?: boolean
  placeholder?: string
  help_url?: string
  doc: string
}

export interface FieldDescriptorSet {
  name: string
  fields: FieldDescriptor[]
}

export function isFieldVisible(
  field: FieldDescriptor,
  values: Record<string, string>,
): boolean {
  return (field.show_when ?? []).every((c) =>
    c.equals.includes(values[c.field] ?? ''),
  )
}
