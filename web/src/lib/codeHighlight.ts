// Tiny line tokenizer for config and command blocks. It only colours, never rewrites text,
// so joining the token texts always gives back the original line.
export type CodeTokenKind =
  | 'plain'
  | 'keyword'
  | 'string'
  | 'number'
  | 'comment'
  | 'function'
  | 'type'
  | 'property'
  | 'punctuation'

export interface CodeToken {
  text: string
  kind: CodeTokenKind
}

export type CodeLanguage = 'yaml' | 'json' | 'shell' | 'diff'

export type DiffLineKind = 'added' | 'removed' | 'hunk' | 'context'

export const TOKEN_CLASS: Record<CodeTokenKind, string> = {
  plain: '',
  keyword: 'text-glinr-code-keyword',
  string: 'text-glinr-code-string',
  number: 'text-glinr-code-number',
  comment: 'text-glinr-code-comment',
  function: 'text-glinr-code-function',
  type: 'text-glinr-code-type',
  property: 'text-glinr-code-property',
  punctuation: 'text-glinr-code-punctuation',
}

const LANGUAGE_ALIASES: Record<string, CodeLanguage> = {
  yaml: 'yaml',
  yml: 'yaml',
  json: 'json',
  bash: 'shell',
  sh: 'shell',
  shell: 'shell',
  zsh: 'shell',
  diff: 'diff',
  patch: 'diff',
}

export function resolveLanguage(name: string | undefined): CodeLanguage | null {
  return name ? (LANGUAGE_ALIASES[name.trim().toLowerCase()] ?? null) : null
}

const SCALAR = /^(true|false|null|yes|no|on|off|~)$/i

function scalarKind(value: string): CodeTokenKind {
  if (SCALAR.test(value)) return 'keyword'
  if (/^-?\d+(\.\d+)?([a-zA-Z%]{0,3})$/.test(value)) return 'number'
  return 'plain'
}

// Splits a scalar tail such as `"x" # note` into value and trailing comment.
function valueTokens(rest: string): CodeToken[] {
  const out: CodeToken[] = []
  const re = /("(?:[^"\\]|\\.)*"|'[^']*')|(\s#.*$)|([^\s"'#]+)|(\s+|.)/g
  let m: RegExpExecArray | null
  while ((m = re.exec(rest)) !== null) {
    if (m[1]) out.push({ text: m[1], kind: 'string' })
    else if (m[2]) out.push({ text: m[2], kind: 'comment' })
    else if (m[3]) {
      const kind = scalarKind(m[3])
      out.push({
        text: m[3],
        kind:
          kind === 'plain' && /^[[\]{},]+$/.test(m[3]) ? 'punctuation' : kind,
      })
    } else out.push({ text: m[0], kind: 'plain' })
  }
  return out
}

function yamlLine(line: string): CodeToken[] {
  if (/^\s*#/.test(line)) return [{ text: line, kind: 'comment' }]
  const key =
    /^(\s*(?:-\s+)*)("[^"]*"|'[^']*'|[^\s:#][^:#]*?)(:)(\s|$)(.*)$/.exec(line)
  if (key) {
    const lead = key[1] ?? ''
    const name = key[2] ?? ''
    const colon = key[3] ?? ''
    const gap = key[4] ?? ''
    const rest = key[5] ?? ''
    return [
      ...(lead ? [{ text: lead, kind: 'punctuation' as const }] : []),
      { text: name, kind: 'property' },
      { text: colon, kind: 'punctuation' },
      { text: gap, kind: 'plain' },
      ...valueTokens(rest),
    ]
  }
  const item = /^(\s*-\s+)(.*)$/.exec(line)
  if (item) {
    return [
      { text: item[1] ?? '', kind: 'punctuation' },
      ...valueTokens(item[2] ?? ''),
    ]
  }
  return valueTokens(line)
}

function jsonLine(line: string): CodeToken[] {
  const out: CodeToken[] = []
  const re =
    /("(?:[^"\\]|\\.)*")(\s*:)?|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|\b(true|false|null)\b|([{}[\],:])|(\s+|.)/g
  let m: RegExpExecArray | null
  while ((m = re.exec(line)) !== null) {
    if (m[1] !== undefined) {
      if (m[2]) {
        out.push({ text: m[1], kind: 'property' })
        out.push({ text: m[2], kind: 'punctuation' })
      } else out.push({ text: m[1], kind: 'string' })
    } else if (m[3] !== undefined) out.push({ text: m[3], kind: 'number' })
    else if (m[4] !== undefined) out.push({ text: m[4], kind: 'keyword' })
    else if (m[5] !== undefined) out.push({ text: m[5], kind: 'punctuation' })
    else out.push({ text: m[0], kind: 'plain' })
  }
  return out
}

function shellLine(line: string): CodeToken[] {
  if (/^\s*#/.test(line)) return [{ text: line, kind: 'comment' }]
  const out: CodeToken[] = []
  const re =
    /("(?:[^"\\]|\\.)*"|'[^']*')|(\s#.*$)|(\$\{?\w+\}?)|(--?[A-Za-z][\w-]*)|([|&;<>]+)|(\s+)|([^\s"'|&;<>]+)/g
  let m: RegExpExecArray | null
  let sawWord = false
  while ((m = re.exec(line)) !== null) {
    if (m[1]) out.push({ text: m[1], kind: 'string' })
    else if (m[2]) out.push({ text: m[2], kind: 'comment' })
    else if (m[3]) out.push({ text: m[3], kind: 'type' })
    else if (m[4]) out.push({ text: m[4], kind: 'property' })
    else if (m[5]) {
      out.push({ text: m[5], kind: 'punctuation' })
      sawWord = false
    } else if (m[6]) out.push({ text: m[6], kind: 'plain' })
    else {
      out.push({ text: m[7] ?? '', kind: sawWord ? 'plain' : 'function' })
      sawWord = true
    }
  }
  return out
}

export function tokenizeLine(line: string, lang: CodeLanguage): CodeToken[] {
  if (line === '') return []
  switch (lang) {
    case 'yaml':
      return yamlLine(line)
    case 'json':
      return jsonLine(line)
    case 'shell':
      return shellLine(line)
    case 'diff':
      return [{ text: line, kind: 'plain' }]
  }
}

export function diffLineKind(line: string): DiffLineKind {
  if (line.startsWith('+++') || line.startsWith('---')) return 'hunk'
  if (line.startsWith('@@')) return 'hunk'
  if (line.startsWith('+')) return 'added'
  if (line.startsWith('-')) return 'removed'
  return 'context'
}

export const DIFF_LINE_CLASS: Record<DiffLineKind, string> = {
  added: 'bg-glinr-code-added-bg text-glinr-code-added',
  removed: 'bg-glinr-code-removed-bg text-glinr-code-removed',
  hunk: 'text-glinr-code-type',
  context: '',
}
