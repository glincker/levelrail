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
