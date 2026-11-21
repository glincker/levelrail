import { describe, expect, it } from 'vitest'
import { detectLogLevel, prettyJsonLine } from './logLevel'

describe('detectLogLevel', () => {
  it.each([
    ['{"level":"error","msg":"x"}', 'error'],
    ['{"severity":"WARNING","msg":"x"}', 'warn'],
    ['{"lvl":"debug"}', 'debug'],
    ['{"msg":"no level"}', null],
    ['ERROR db timeout', 'error'],
    ['WARN: disk low', 'warn'],
    ['info starting', 'info'],
    ['[DEBUG] cache miss', 'debug'],
    ['\u001b[31mFATAL\u001b[0m boom', 'error'],
    ['time=2026-01-01 level=WARN msg="slow"', 'warn'],
    ['level="error" msg=x', 'error'],
    ['server started', null],
    ['informational text', null],
    ['', null],
  ])('%s -> %s', (line, want) => {
    expect(detectLogLevel(line)).toBe(want)
  })
})

describe('prettyJsonLine', () => {
  it('pretty prints objects', () => {
    expect(prettyJsonLine('{"a":1}')).toBe('{\n  "a": 1\n}')
  })
  it.each(['plain', '{bad json}', '[1,2]', ''])('null for %s', (l) => {
    expect(prettyJsonLine(l)).toBeNull()
  })
})

describe('detectLogLevel bracketed server levels', () => {
  it('reads a bracketed notice as info', () => {
    expect(
      detectLogLevel('2026/11/21 04:10:37 [notice] 1#1: start worker'),
    ).toBe('info')
  })
  it('reads a bracketed emerg as error', () => {
    expect(detectLogLevel('2026/11/21 04:10:37 [emerg] 1#1: bind failed')).toBe(
      'error',
    )
  })
})
