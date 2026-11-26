// Conservative SQL formatter: uppercases common keywords and starts the
// main clauses of top-level statements on their own line. Strings,
// quoted identifiers and comments pass through untouched; anything with a
// dollar-quoted body is returned as is.

const KEYWORDS = new Set(
  (
    'select from where and or not in is null as on join left right inner outer full cross natural ' +
    'group by order having limit offset union intersect except all distinct insert into values ' +
    'update set delete create table alter drop with returning case when then else end asc desc ' +
    'like ilike between exists true false using'
  ).split(' '),
)

// Words that open a new line when they appear outside parentheses.
const CLAUSE = new Set(
  (
    'from where group order having limit offset union intersect except values set returning ' +
    'left right inner full cross join'
  ).split(' '),
)
const JOIN_MODS = new Set([
  'left',
  'right',
  'inner',
  'full',
  'cross',
  'outer',
  'natural',
])

type Token = { kind: 'ws' | 'text' | 'word' | 'lineComment'; value: string }

const TOKEN =
  /(\s+)|(--[^\n]*)|(\/\*[\s\S]*?\*\/|'(?:[^']|'')*'|"(?:[^"]|"")*"|`[^`]*`)|([A-Za-z_][A-Za-z0-9_$]*)|([\s\S])/gy

function tokenize(sql: string): Token[] {
  const out: Token[] = []
  TOKEN.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = TOKEN.exec(sql)) !== null) {
    if (m[1] !== undefined) out.push({ kind: 'ws', value: m[1] })
    else if (m[2] !== undefined) out.push({ kind: 'lineComment', value: m[2] })
    else if (m[3] !== undefined) out.push({ kind: 'text', value: m[3] })
    else if (m[4] !== undefined) out.push({ kind: 'word', value: m[4] })
    else out.push({ kind: 'text', value: m[5] ?? '' })
  }
  return out
}

export function formatSql(sql: string): string {
  if (sql.includes('$$')) {
    return sql
  }
  const tokens = tokenize(sql)
  let out = ''
  let depth = 0
  let prevWord = ''
  const nextSignificant = (from: number): string => {
    for (let i = from; i < tokens.length; i++) {
      const t = tokens[i] as Token
      if (t.kind !== 'ws') return t.value
    }
    return ''
  }
  const atLineStart = () => out === '' || out.endsWith('\n')

  tokens.forEach((tok, i) => {
    if (tok.kind === 'ws') {
      if (!atLineStart() && !out.endsWith(' ')) out += ' '
      return
    }
    if (tok.kind === 'lineComment') {
      out += tok.value + '\n'
      return
    }
    if (tok.kind === 'text') {
      if (tok.value === '(') depth++
      if (tok.value === ')') depth = Math.max(0, depth - 1)
      out += tok.value
      if (tok.value === ';' && depth === 0) out += '\n\n'
      prevWord = ''
      return
    }
    const lower = tok.value.toLowerCase()
    let startsLine = depth === 0 && CLAUSE.has(lower)
    if (startsLine && lower === 'join' && JOIN_MODS.has(prevWord))
      startsLine = false
    if (
      startsLine &&
      (lower === 'left' || lower === 'right') &&
      nextSignificant(i + 1) === '('
    ) {
      startsLine = false
    }
    if (
      startsLine &&
      (prevWord === 'delete' || prevWord === 'distinct' || prevWord === 'is')
    ) {
      startsLine = false
    }
    if (startsLine && !atLineStart()) {
      out = out.replace(/ +$/, '') + '\n'
    }
    out += KEYWORDS.has(lower) ? lower.toUpperCase() : tok.value
    prevWord = lower
  })
  return out.replace(/ +\n/g, '\n').trim() + (sql.endsWith('\n') ? '\n' : '')
}
