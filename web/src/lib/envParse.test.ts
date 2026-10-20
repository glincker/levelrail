import { describe, expect, it } from 'vitest'
import {
  classifyEnvImport,
  diffEnv,
  formatEnvExport,
  parseEnvBlock,
} from './envParse'

describe('parseEnvBlock', () => {
  const cases: [string, string, { key: string; value: string }[]][] = [
    ['empty', '', []],
    ['comments and blanks', '# c\n\n  # x\nA=1', [{ key: 'A', value: '1' }]],
    [
      'export prefix',
      'export A=1\nexport   B=2',
      [
        { key: 'A', value: '1' },
        { key: 'B', value: '2' },
      ],
    ],
    [
      'equals inside value',
      'URL=postgres://u:p@h/db?a=b&c=d',
      [{ key: 'URL', value: 'postgres://u:p@h/db?a=b&c=d' }],
    ],
    [
      'empty values',
      'A=\nB=""\nC=\'\'',
      [
        { key: 'A', value: '' },
        { key: 'B', value: '' },
        { key: 'C', value: '' },
      ],
    ],
    [
      'duplicate keys preserved',
      'A=1\nA=2',
      [
        { key: 'A', value: '1' },
        { key: 'A', value: '2' },
      ],
    ],
    [
      'windows line endings',
      'A=1\r\nB="two"\r\n',
      [
        { key: 'A', value: '1' },
        { key: 'B', value: 'two' },
      ],
    ],
    ['bom', '﻿A=1', [{ key: 'A', value: '1' }]],
    [
      'double quote escapes',
      'A="l1\\nl2 \\"q\\" \\\\"',
      [{ key: 'A', value: 'l1\nl2 "q" \\' }],
    ],
    ['single quotes literal', "A='a\\nb'", [{ key: 'A', value: 'a\\nb' }]],
    [
      'multiline double',
      'A="l1\nl2"\nB=2',
      [
        { key: 'A', value: 'l1\nl2' },
        { key: 'B', value: '2' },
      ],
    ],
    [
      'multiline single with crlf',
      "A='l1\r\nl2'\r\nB=2",
      [
        { key: 'A', value: 'l1\nl2' },
        { key: 'B', value: '2' },
      ],
    ],
    [
      'inline comment',
      'A=val # note\nB=a#b',
      [
        { key: 'A', value: 'val' },
        { key: 'B', value: 'a#b' },
      ],
    ],
    ['hash inside quotes', 'A="x # y" # tail', [{ key: 'A', value: 'x # y' }]],
    [
      'unmatched quote left as is',
      "A='\nB=2",
      [
        { key: 'A', value: "'" },
        { key: 'B', value: '2' },
      ],
    ],
    [
      'invalid keys skipped',
      'no-equals\n=nokey\n1BAD=x\nGOOD_1=y',
      [{ key: 'GOOD_1', value: 'y' }],
    ],
    ['spaces around equals', 'A = b ', [{ key: 'A', value: 'b' }]],
  ]
  it.each(cases)('%s', (_name, input, want) => {
    expect(parseEnvBlock(input)).toEqual(want)
  })
})

describe('classifyEnvImport', () => {
  it('classifies new, changed and unchanged, last duplicate wins', () => {
    const rows = classifyEnvImport({ A: '1', B: 'old' }, [
      { key: 'A', value: '1' },
      { key: 'B', value: 'new' },
      { key: 'C', value: '3' },
      { key: 'C', value: '4' },
    ])
    expect(rows).toEqual([
      { key: 'A', value: '1', status: 'unchanged' },
      { key: 'B', value: 'new', status: 'changed', previous: 'old' },
      { key: 'C', value: '4', status: 'new' },
    ])
  })
})

describe('diffEnv', () => {
  it('reports added, changed and removed keys', () => {
    expect(
      diffEnv({ A: '1', B: '2', C: '3' }, { A: '1', B: 'x', D: '4' }),
    ).toEqual({
      added: ['D'],
      changed: ['B'],
      removed: ['C'],
    })
  })
})

describe('formatEnvExport', () => {
  it('never exports secret values and round-trips plain ones', () => {
    const env = {
      PLAIN: 'v',
      SPACED: 'a b',
      MULTI: 'l1\nl2',
      QUOTES: 'say "hi"',
      EMPTY: '',
      API_KEY: 'must-not-leak',
    }
    const out = formatEnvExport(env, ['API_KEY', 'STORE_ONLY'])
    expect(out).not.toContain('must-not-leak')
    expect(out).toContain('# secret, value not exported\nAPI_KEY=\n')
    expect(out).toContain('STORE_ONLY=\n')
    const back = Object.fromEntries(
      parseEnvBlock(out).map((e) => [e.key, e.value]),
    )
    for (const k of ['PLAIN', 'SPACED', 'MULTI', 'QUOTES', 'EMPTY'] as const) {
      expect(back[k]).toBe(env[k])
    }
  })
})
