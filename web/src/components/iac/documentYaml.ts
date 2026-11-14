// Minimal YAML for the IaC document subset (maps, lists, scalars, flow collections); no dependency.
import {
  isSecretLikeName,
  type IacField,
  type IacKindModel,
} from './schemaToFields'

export interface KvEntry {
  key: string
  value: string
  secret: boolean
}
export type FieldValue = string | boolean | string[] | KvEntry[]
export type FormValues = Record<string, FieldValue>
type Tree = { [k: string]: unknown }

export class YamlParseError extends Error {
  readonly line: number
  constructor(message: string, line: number) {
    super(message)
    this.line = line
  }
}

// ---------- serialize ----------

const PLAIN_SAFE = /^[A-Za-z_][A-Za-z0-9_./@-]*$/

function scalar(v: unknown): string {
  if (typeof v === 'string') {
    const reserved = /^(true|false|null|yes|no|on|off|~)$/i.test(v)
    return PLAIN_SAFE.test(v) && !reserved ? v : JSON.stringify(v)
  }
  return String(v)
}

function emit(v: unknown, indent: number, out: string[]) {
  const pad = ' '.repeat(indent)
  if (Array.isArray(v)) {
    for (const item of v) {
      if (item && typeof item === 'object') {
        const sub: string[] = []
        emit(item, indent + 2, sub)
        out.push(`${pad}- ${(sub[0] ?? '').trimStart()}`, ...sub.slice(1))
      } else out.push(`${pad}- ${scalar(item)}`)
    }
    return
  }
  for (const [k, val] of Object.entries(v as Tree)) {
    const key = scalar(k)
    if (Array.isArray(val) && val.length === 0) out.push(`${pad}${key}: []`)
    else if (val && typeof val === 'object') {
      if (Object.keys(val).length === 0) out.push(`${pad}${key}: {}`)
      else {
        out.push(`${pad}${key}:`)
        emit(val, indent + 2, out)
      }
    } else out.push(`${pad}${key}: ${scalar(val)}`)
  }
}

export function toYaml(tree: Tree): string {
  const out: string[] = []
  emit(tree, 0, out)
  return `${out.join('\n')}\n`
}

// ---------- parse ----------

interface Line {
  indent: number
  text: string
  no: number
}

function stripComment(s: string): string {
  let q = ''
  for (let i = 0; i < s.length; i++) {
    const c = s[i]
    if (q) {
      if (c === '\\' && q === '"') i++
      else if (c === q) q = ''
    } else if (c === '"' || c === "'") {
      if (i === 0 || /[\s[{,:]/.test(s[i - 1] ?? '')) q = c
    } else if (c === '#' && (i === 0 || /\s/.test(s[i - 1] ?? ''))) {
      return s.slice(0, i).trimEnd()
    }
  }
  return s.trimEnd()
}

function plain(raw: string): unknown {
  if (raw === 'true') return true
  if (raw === 'false') return false
  if (raw === 'null' || raw === '~' || raw === '') return null
  if (/^-?\d+(\.\d+)?$/.test(raw)) return Number(raw)
  return raw
}

function quoted(raw: string, no: number): string {
  if (raw.startsWith('"')) {
    try {
      return JSON.parse(raw) as string
    } catch {
      throw new YamlParseError('bad quoted string', no)
    }
  }
  return raw.slice(1, -1).replace(/''/g, "'")
}

function flow(src: string, no: number): unknown {
  let i = 0
  const ws = () => {
    while (src[i] === ' ') i++
  }
  const item = (): unknown => {
    ws()
    const c = src[i]
    if (c === '[') {
      i++
      const arr: unknown[] = []
      ws()
      while (src[i] !== ']') {
        arr.push(item())
        ws()
        if (src[i] === ',') i++
        else if (src[i] !== ']') throw new YamlParseError('bad flow list', no)
        ws()
      }
      i++
      return arr
    }
    if (c === '{') {
      i++
      const obj: Tree = {}
      ws()
      while (src[i] !== '}') {
        const k = item()
        ws()
        if (src[i] !== ':') throw new YamlParseError('bad flow map', no)
        i++
        obj[String(k)] = item()
        ws()
        if (src[i] === ',') i++
        else if (src[i] !== '}') throw new YamlParseError('bad flow map', no)
        ws()
      }
      i++
      return obj
    }
    if (c === '"' || c === "'") {
      let j = i + 1
      while (
        j < src.length &&
        (src[j] !== c || (c === '"' && src[j - 1] === '\\'))
      )
        j++
      const lit = src.slice(i, j + 1)
      i = j + 1
      return quoted(lit, no)
    }
    let j = i
    while (j < src.length && !',]}:'.includes(src[j] ?? '')) j++
    const lit = src.slice(i, j).trim()
    i = j
    return plain(lit)
  }
  const v = item()
  ws()
  if (i < src.length) throw new YamlParseError('bad flow value', no)
  return v
}

function inline(raw: string, no: number): unknown {
  const s = stripComment(raw.trim())
  if (s.startsWith('[') || s.startsWith('{')) return flow(s, no)
  if (s.startsWith('"') || s.startsWith("'")) {
    if (s.length < 2 || !s.endsWith(s[0] ?? ''))
      throw new YamlParseError('bad quote', no)
    return quoted(s, no)
  }
  if (/^[|>]/.test(s))
    throw new YamlParseError('block scalars are not supported', no)
  return plain(s)
}

function splitKey(
  text: string,
  no: number,
): { key: string; rest: string } | null {
  if (text.startsWith('[') || text.startsWith('{')) return null
  let end = -1
  if (text.startsWith('"') || text.startsWith("'")) {
    const q = text[0] as string
    let j = 1
    while (
      j < text.length &&
      (text[j] !== q || (q === '"' && text[j - 1] === '\\'))
    )
      j++
    if (text[j + 1] === ':' && (j + 2 >= text.length || text[j + 2] === ' ')) {
      return {
        key: quoted(text.slice(0, j + 1), no),
        rest: text.slice(j + 2).trim(),
      }
    }
    return null
  }
  for (let i = 0; i < text.length; i++) {
    if (text[i] === ':' && (i === text.length - 1 || text[i + 1] === ' ')) {
      end = i
      break
    }
  }
  if (end < 0) return null
  return { key: text.slice(0, end).trim(), rest: text.slice(end + 1).trim() }
}

export function parseYaml(src: string): unknown {
  const lines: Line[] = []
  src.split(/\r?\n/).forEach((raw, i) => {
    if (/^\s*\t/.test(raw) && raw.trim())
      throw new YamlParseError('tabs', i + 1)
    const text = stripComment(raw.trimStart())
    if (!text || text === '---') return
    lines.push({ indent: raw.length - raw.trimStart().length, text, no: i + 1 })
  })
  let pos = 0
  const isSeq = (l: Line) => l.text === '-' || l.text.startsWith('- ')

  const block = (indent: number): unknown => {
    const l = lines[pos]
    if (!l) return null
    return isSeq(l) ? seq(indent) : map(indent)
  }
  const child = (parentIndent: number, allowSameSeq: boolean): unknown => {
    const n = lines[pos]
    if (n && n.indent > parentIndent) return block(n.indent)
    if (allowSameSeq && n && n.indent === parentIndent && isSeq(n))
      return seq(parentIndent)
    return null
  }
  const map = (indent: number): Tree => {
    const out: Tree = {}
    while (pos < lines.length) {
      const l = lines[pos] as Line
      if (l.indent !== indent || isSeq(l)) break
      const kv = splitKey(l.text, l.no)
      if (!kv) throw new YamlParseError('expected key: value', l.no)
      pos++
      out[kv.key] = kv.rest ? inline(kv.rest, l.no) : child(indent, true)
    }
    return out
  }
  const seq = (indent: number): unknown[] => {
    const out: unknown[] = []
    while (pos < lines.length) {
      const l = lines[pos] as Line
      if (l.indent !== indent || !isSeq(l)) break
      const rest = l.text.slice(1).trimStart()
      if (!rest) {
        pos++
        out.push(child(indent, false))
      } else if (splitKey(rest, l.no)) {
        lines[pos] = {
          indent: indent + (l.text.length - rest.length),
          text: rest,
          no: l.no,
        }
        out.push(map(indent + (l.text.length - rest.length)))
      } else {
        pos++
        out.push(inline(rest, l.no))
      }
    }
    return out
  }
  if (lines.length === 0) return null
  const first = lines[0] as Line
  const result = block(first.indent)
  if (pos < lines.length) {
    throw new YamlParseError('unexpected indentation', (lines[pos] as Line).no)
  }
  return result
}

// ---------- values <-> document ----------

function setPath(tree: Tree, path: string[], val: unknown) {
  let cur = tree
  path.slice(0, -1).forEach((p) => {
    const next = cur[p]
    if (!next || typeof next !== 'object' || Array.isArray(next)) cur[p] = {}
    cur = cur[p] as Tree
  })
  cur[path[path.length - 1] as string] = val
}

function getPath(tree: Tree, path: string[]): unknown {
  let cur: unknown = tree
  for (const p of path) {
    if (!cur || typeof cur !== 'object' || Array.isArray(cur)) return undefined
    cur = (cur as Tree)[p]
  }
  return cur
}

function deletePath(tree: Tree, path: string[]) {
  const parent = getPath(tree, path.slice(0, -1))
  if (parent && typeof parent === 'object' && !Array.isArray(parent)) {
    delete (parent as Tree)[path[path.length - 1] as string]
  }
}

function pruneEmpty(v: unknown): unknown {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return v
  const t = v as Tree
  for (const k of Object.keys(t)) {
    const c = pruneEmpty(t[k])
    if (
      c &&
      typeof c === 'object' &&
      !Array.isArray(c) &&
      Object.keys(c).length === 0
    ) {
      delete t[k]
    }
  }
  return t
}

function mergeInto(dst: Tree, src: Tree) {
  for (const [k, v] of Object.entries(src)) {
    const d = dst[k]
    if (
      v &&
      typeof v === 'object' &&
      !Array.isArray(v) &&
      d &&
      typeof d === 'object' &&
      !Array.isArray(d)
    ) {
      mergeInto(d as Tree, v as Tree)
    } else dst[k] = v
  }
}

// Raw secret values never leave the form: only a secretRef name is ever emitted.
function fieldToYaml(f: IacField, v: FieldValue | undefined): unknown {
  if (v === undefined) return undefined
  switch (f.type) {
    case 'boolean':
      return typeof v === 'boolean' ? v : undefined
    case 'stringList': {
      const items = Array.isArray(v)
        ? (v as string[]).map((s) => s.trim()).filter(Boolean)
        : []
      return items.length ? items : undefined
    }
    case 'keyValue': {
      const out: Tree = {}
      for (const e of Array.isArray(v) ? (v as KvEntry[]) : []) {
        const k = e.key.trim()
        const val = e.value.trim()
        if (!k || !val) continue
        if (e.secret) {
          if (f.secretRefs) out[k] = { secretRef: val }
        } else if (!isSecretLikeName(k)) out[k] = val
      }
      return Object.keys(out).length ? out : undefined
    }
    default: {
      const s = typeof v === 'string' ? v.trim() : ''
      if (!s) return undefined
      if (f.secret) return { secretRef: s }
      if (f.type === 'number') {
        const n = Number(s)
        return Number.isFinite(n) ? n : undefined
      }
      return s
    }
  }
}

export function serializeDocument(
  model: IacKindModel,
  values: FormValues,
  extra: Tree = {},
): string {
  const tree: Tree = { version: 1, kind: model.kind }
  for (const f of model.fields) {
    if (f.type === 'group') continue
    const out = fieldToYaml(f, values[f.id])
    if (out !== undefined) setPath(tree, f.path, out)
  }
  const spec = tree.spec
  if (
    model.fields.some((f) => f.path[0] === 'spec') &&
    (!spec || typeof spec !== 'object')
  ) {
    tree.spec = {}
  }
  mergeInto(tree, extra)
  return toYaml(tree)
}

export interface ReadDocument {
  kind: string | null
  tree: Tree
}

export function readDocument(yaml: string): ReadDocument {
  const raw = parseYaml(yaml)
  if (raw === null) return { kind: null, tree: {} }
  if (typeof raw !== 'object' || Array.isArray(raw)) {
    throw new YamlParseError('document must be a map', 1)
  }
  const tree = raw as Tree
  return { kind: typeof tree.kind === 'string' ? tree.kind : null, tree }
}

export function valuesFromTree(
  model: IacKindModel,
  source: Tree,
): { values: FormValues; extra: Tree } {
  const tree = structuredClone(source)
  const values: FormValues = {}
  for (const f of model.fields) {
    if (f.type === 'group') continue
    const raw = getPath(tree, f.path)
    if (raw === undefined || raw === null) continue
    if (f.type === 'boolean' && typeof raw === 'boolean') values[f.id] = raw
    else if (
      f.type === 'stringList' &&
      Array.isArray(raw) &&
      raw.every((x) => typeof x === 'string' || typeof x === 'number')
    ) {
      values[f.id] = raw.map(String)
    } else if (
      f.type === 'keyValue' &&
      typeof raw === 'object' &&
      !Array.isArray(raw)
    ) {
      const entries: KvEntry[] = []
      for (const [k, val] of Object.entries(raw as Tree)) {
        const ref = val && typeof val === 'object' ? (val as Tree) : null
        if (
          ref &&
          Object.keys(ref).length === 1 &&
          typeof ref.secretRef === 'string'
        ) {
          entries.push({ key: k, value: ref.secretRef, secret: true })
        } else if (
          typeof val === 'string' ||
          typeof val === 'number' ||
          typeof val === 'boolean'
        ) {
          entries.push({ key: k, value: String(val), secret: false })
        } else continue
        delete (raw as Tree)[k]
      }
      values[f.id] = entries
      if (Object.keys(raw).length > 0) continue
    } else if (
      f.secret &&
      typeof raw === 'object' &&
      !Array.isArray(raw) &&
      typeof (raw as Tree).secretRef === 'string'
    ) {
      values[f.id] = (raw as Tree).secretRef as string
    } else if (
      (f.type === 'string' || f.type === 'enum' || f.type === 'number') &&
      (typeof raw === 'string' ||
        typeof raw === 'number' ||
        typeof raw === 'boolean')
    ) {
      values[f.id] = f.secret ? '' : String(raw)
      if (f.secret) continue
    } else continue
    deletePath(tree, f.path)
  }
  delete tree.version
  delete tree.kind
  return { values, extra: pruneEmpty(tree) as Tree }
}
