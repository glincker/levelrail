import { describe, expect, it } from 'vitest'
import {
  diffLineKind,
  resolveLanguage,
  tokenizeLine,
  type CodeLanguage,
} from './codeHighlight'

const join = (line: string, lang: CodeLanguage) =>
  tokenizeLine(line, lang)
    .map((t) => t.text)
    .join('')

describe('codeHighlight', () => {
  it('never changes the text of a line', () => {
    const cases: [string, CodeLanguage][] = [
      ['  - name: "web" # note', 'yaml'],
      ['key: true', 'yaml'],
      ['{"a": [1, 2.5e3, null], "b": "x"}', 'json'],
      ['levelrail deploy --app web | tee "$OUT" # go', 'shell'],
      ['+added', 'diff'],
    ]
    for (const [line, lang] of cases) expect(join(line, lang)).toBe(line)
  })

  it('classifies yaml keys, scalars and comments', () => {
    const kinds = tokenizeLine('replicas: 3 # scale', 'yaml').map((t) => t.kind)
    expect(kinds).toContain('property')
    expect(kinds).toContain('number')
    expect(kinds).toContain('comment')
  })

  it('classifies json properties apart from string values', () => {
    const tokens = tokenizeLine('"a": "b"', 'json')
    expect(tokens[0]).toEqual({ text: '"a"', kind: 'property' })
    expect(tokens.find((t) => t.text === '"b"')?.kind).toBe('string')
  })

  it('marks the command word and flags in shell', () => {
    const tokens = tokenizeLine('levelrail apps --json', 'shell')
    expect(tokens[0]?.kind).toBe('function')
    expect(tokens.at(-1)?.kind).toBe('property')
  })

  it('resolves language aliases and diff line kinds', () => {
    expect(resolveLanguage('YML')).toBe('yaml')
    expect(resolveLanguage('bash')).toBe('shell')
    expect(resolveLanguage('go')).toBeNull()
    expect(diffLineKind('+a')).toBe('added')
    expect(diffLineKind('-a')).toBe('removed')
    expect(diffLineKind('--- a/x')).toBe('hunk')
    expect(diffLineKind(' ctx')).toBe('context')
  })
})
