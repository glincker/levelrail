// Pure model: turns one kind of the IaC resources JSON Schema into a flat, typed field list.

export interface SchemaNode {
  type?: string | string[]
  description?: string
  enum?: unknown[]
  default?: unknown
  properties?: Record<string, SchemaNode>
  required?: string[]
  items?: SchemaNode
  additionalProperties?: boolean | SchemaNode
  oneOf?: SchemaNode[]
  $ref?: string
  minimum?: number
  maximum?: number
  minLength?: number
  pattern?: string
  'x-stability'?: string
}

export interface IacSchema extends Omit<SchemaNode, 'x-stability'> {
  'x-stability'?: Record<string, string>
  $defs?: Record<string, SchemaNode>
  allOf?: Array<{
    if?: { properties?: { kind?: { const?: string } } }
    then?: SchemaNode
  }>
}

export type IacFieldType =
  'string' | 'number' | 'boolean' | 'enum' | 'stringList' | 'keyValue' | 'group'

export interface IacField {
  id: string
  path: string[]
  key: string
  label: string
  description?: string
  type: IacFieldType
  required: boolean
  // Every ancestor is required too, so the field is needed in any valid document.
  effectiveRequired: boolean
  advanced: boolean
  secret: boolean
  default?: string | number | boolean
  options?: string[]
  integer?: boolean
  min?: number
  max?: number
  minLength?: number
  pattern?: string
  // keyValue only: whether a value may be a secretRef object (env maps) or a literal only.
  secretRefs?: boolean
  depth: number
}

export interface IacKindModel {
  kind: string
  stability: string
  fields: IacField[]
  // Schema paths the form cannot edit (object lists, unions); the YAML keeps them untouched.
  unsupported: string[]
}

export const DEFAULT_IAC_KINDS = [
  'App',
  'Database',
  'Domain',
  'Environment',
  'Project',
  'Tag',
] as const

const SECRET_NAME =
  /(password|passwd|token|secret|api[_-]?key|private[_-]?key)/i

export function isSecretLikeName(name: string): boolean {
  return SECRET_NAME.test(name)
}

export function labelFromName(name: string): string {
  const spaced = name
    .replace(/[_-]+/g, ' ')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .trim()
    .toLowerCase()
  return spaced.charAt(0).toUpperCase() + spaced.slice(1)
}

function resolve(schema: IacSchema, node: SchemaNode, guard = 0): SchemaNode {
  if (!node.$ref || guard > 8) return node
  const { $ref, ...local } = node
  const parts = $ref.replace(/^#\//, '').split('/')
  let cur: unknown = schema
  for (const part of parts) {
    if (typeof cur !== 'object' || cur === null) return local
    cur = (cur as Record<string, unknown>)[part]
  }
  if (typeof cur !== 'object' || cur === null) return local
  return resolve(schema, { ...(cur as SchemaNode), ...local }, guard + 1)
}

function typeOf(node: SchemaNode): string | undefined {
  return Array.isArray(node.type) ? node.type[0] : node.type
}

function scalarDefault(v: unknown): string | number | boolean | undefined {
  return typeof v === 'string' ||
    typeof v === 'number' ||
    typeof v === 'boolean'
    ? v
    : undefined
}

interface Ctx {
  schema: IacSchema
  fields: IacField[]
  unsupported: string[]
  maxDepth: number
}

function constraints(node: SchemaNode) {
  return {
    min: node.minimum,
    max: node.maximum,
    minLength: node.minLength,
    pattern: node.pattern,
  }
}

function leafType(
  ctx: Ctx,
  node: SchemaNode,
): Pick<IacField, 'type' | 'options' | 'secretRefs' | 'integer'> | null {
  if (
    Array.isArray(node.enum) &&
    node.enum.every((e) => typeof e === 'string')
  ) {
    return { type: 'enum', options: node.enum }
  }
  const t = typeOf(node)
  if (t === 'boolean') return { type: 'boolean' }
  if (t === 'integer') return { type: 'number', integer: true }
  if (t === 'number') return { type: 'number' }
  if (t === 'string') return { type: 'string' }
  if (t === 'array') {
    const items = node.items ? resolve(ctx.schema, node.items) : undefined
    return items && typeOf(items) === 'string' ? { type: 'stringList' } : null
  }
  if (t === 'object' && node.additionalProperties && !node.properties) {
    const ap = node.additionalProperties
    if (ap === true) return null
    const v = resolve(ctx.schema, ap)
    if (typeOf(v) === 'string') return { type: 'keyValue', secretRefs: false }
    const first = v.oneOf?.[0] ? resolve(ctx.schema, v.oneOf[0]) : undefined
    if (first && typeOf(first) === 'string') {
      return { type: 'keyValue', secretRefs: true }
    }
  }
  return null
}

function walk(
  ctx: Ctx,
  props: Record<string, SchemaNode>,
  required: string[],
  parent: string[],
  parentRequired: boolean,
  baseAdvanced: boolean,
  depth: number,
) {
  for (const key of Object.keys(props).sort()) {
    const raw = props[key]
    if (!raw) continue
    const node = resolve(ctx.schema, raw)
    const path = [...parent, key]
    const id = path.join('.')
    const isRequired = required.includes(key)
    const eff = parentRequired && isRequired
    const base = {
      id,
      path,
      key,
      label: labelFromName(key),
      description: raw.description ?? node.description,
      required: isRequired,
      effectiveRequired: eff,
      secret: isSecretLikeName(key),
      default: scalarDefault(node.default),
      depth,
      ...constraints(node),
    }
    // Required fields and the first optional level of a required group stay visible.
    const advanced = baseAdvanced || (!eff && !(depth <= 1 && parentRequired))
    if (typeOf(node) === 'object' && node.properties && !leafType(ctx, node)) {
      if (depth >= ctx.maxDepth) {
        ctx.unsupported.push(id)
        continue
      }
      ctx.fields.push({ ...base, type: 'group', advanced, secret: false })
      walk(
        ctx,
        node.properties,
        node.required ?? [],
        path,
        eff,
        advanced && !eff,
        depth + 1,
      )
      continue
    }
    const leaf = leafType(ctx, node)
    if (!leaf) {
      ctx.unsupported.push(id)
      continue
    }
    ctx.fields.push({ ...base, ...leaf, advanced })
  }
}

export function specRefFor(schema: IacSchema, kind: string): SchemaNode | null {
  const hit = schema.allOf?.find((a) => a.if?.properties?.kind?.const === kind)
  return hit?.then?.properties?.spec ?? null
}

export function schemaToFields(
  schema: IacSchema,
  kind: string,
  maxDepth = 3,
): IacKindModel {
  const ctx: Ctx = { schema, fields: [], unsupported: [], maxDepth }
  const meta = resolve(schema, schema.$defs?.metadata ?? {})
  walk(
    ctx,
    meta.properties ?? {},
    meta.required ?? [],
    ['metadata'],
    true,
    false,
    0,
  )
  // Name leads the list, labels follow.
  ctx.fields.sort((a, b) => Number(b.required) - Number(a.required))
  const hit = schema.allOf?.find((a) => a.if?.properties?.kind?.const === kind)
  const specNode = hit?.then?.properties?.spec
  const specRequired = hit?.then?.required?.includes('spec') ?? false
  if (specNode) {
    const spec = resolve(schema, specNode)
    if (spec.properties) {
      walk(
        ctx,
        spec.properties,
        spec.required ?? [],
        ['spec'],
        specRequired,
        false,
        0,
      )
    }
  }
  return {
    kind,
    stability: schema['x-stability']?.[kind] ?? 'stable',
    fields: ctx.fields,
    unsupported: ctx.unsupported,
  }
}
