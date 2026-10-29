interface SchemaNode {
  [extra: string]: unknown
  $ref?: string
  description?: string
  default?: unknown
  properties?: Record<string, SchemaNode>
  additionalProperties?: SchemaNode | boolean
  items?: SchemaNode
  allOf?: Array<{
    if?: { properties?: { kind?: { const?: string } } }
    then?: { properties?: Record<string, SchemaNode> }
  }>
  $defs?: Record<string, SchemaNode>
}

export interface FieldHelp {
  description: string
  defaultValue?: string
}

function deref(
  root: SchemaNode,
  node: SchemaNode | undefined,
): SchemaNode | undefined {
  if (!node?.$ref) return node
  const name = node.$ref.replace('#/$defs/', '')
  const target = root.$defs?.[name]
  return target
    ? { ...target, description: node.description ?? target.description }
    : undefined
}

function child(
  root: SchemaNode,
  node: SchemaNode,
  key: string,
): SchemaNode | undefined {
  const props = node.properties?.[key]
  if (props) return deref(root, props)
  if (
    node.additionalProperties &&
    typeof node.additionalProperties === 'object'
  ) {
    return deref(root, node.additionalProperties)
  }
  return undefined
}

// Walks a YAML key path ("spec", "health", "readyTimeout") to its schema node.
export function resolveFieldHelp(
  root: SchemaNode,
  kind: string,
  path: string[],
): FieldHelp | null {
  let node: SchemaNode | undefined = root
  for (let i = 0; i < path.length && node; i++) {
    const key = path[i]
    if (key === undefined) break
    if (i === 0 && key === 'spec') {
      const rule = root.allOf?.find(
        (r) => r.if?.properties?.kind?.const === kind,
      )
      node = deref(root, rule?.then?.properties?.spec ?? root.properties?.spec)
      continue
    }
    node = child(root, node, key)
    if (node?.items && i < path.length - 1) node = deref(root, node.items)
  }
  if (!node?.description) return null
  return {
    description: node.description,
    defaultValue:
      node.default === undefined ? undefined : JSON.stringify(node.default),
  }
}

// Key path of a YAML line from indentation; list dashes count as indent.
export function yamlPathAtLine(
  lines: string[],
  index: number,
): { kind: string; path: string[] } {
  const keyOf = (l: string) => /^(\s*(?:-\s+)*)([A-Za-z0-9_.-]+):/.exec(l)
  const cur = keyOf(lines[index] ?? '')
  if (!cur) return { kind: '', path: [] }
  let start = index
  while (start > 0 && (lines[start - 1] ?? '').trim() !== '---') start--
  const kindLine = lines.slice(start, index + 1).find((l) => /^kind:/.test(l))
  const kind = kindLine ? kindLine.replace(/^kind:\s*/, '').trim() : ''
  const path = [cur[2] ?? '']
  let indent = (cur[1] ?? '').length
  for (let i = index - 1; i >= start && indent > 0; i--) {
    const m = keyOf(lines[i] ?? '')
    const mIndent = (m?.[1] ?? '').length
    if (m && mIndent < indent) {
      path.unshift(m[2] ?? '')
      indent = mIndent
    }
  }
  return { kind, path }
}
