export type TokenKind =
  'comment' | 'string' | 'key' | 'number' | 'keyword' | 'plain'

export interface Token {
  text: string
  kind: TokenKind
}

const RE =
  /(#[^\n]*|\/\/[^\n]*)|("(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*')|(\b\d+(?:\.\d+)?[a-z]*\b)|(\b(?:resource|variable|const|new|import|from|export|let|var|true|false|null|handle|reverse_proxy|Type|Properties)\b)|([A-Za-z_][\w.-]*)(?=\s*[:=])/g

// Small regex tokenizer shared by Terraform, TypeScript, YAML/JSON and Caddyfile.
export function tokenize(source: string): Token[] {
  const out: Token[] = []
  let last = 0
  for (const m of source.matchAll(RE)) {
    const start = m.index ?? 0
    if (start > last)
      out.push({ text: source.slice(last, start), kind: 'plain' })
    const text = m[0]
    let kind: TokenKind = 'plain'
    if (m[1]) kind = 'comment'
    else if (m[2])
      kind = /^\s*:/.test(source.slice(start + text.length)) ? 'key' : 'string'
    else if (m[3]) kind = 'number'
    else if (m[4]) kind = 'keyword'
    else if (m[5]) kind = 'key'
    out.push({ text, kind })
    last = start + text.length
  }
  if (last < source.length)
    out.push({ text: source.slice(last), kind: 'plain' })
  return out
}

export const TOKEN_CLASS: Record<TokenKind, string> = {
  comment: 'text-muted-foreground italic',
  string: 'text-tone-success',
  key: 'text-tone-info',
  number: 'text-tone-warning',
  keyword: 'text-tone-accent font-medium',
  plain: '',
}
