import { describe, expect, it } from 'vitest'
import type { LogLine } from '../hooks/useLogStream'
import {
  filterLogLines,
  findFirstErrorIndex,
  logLinesToText,
} from './logFilter'

const lines: LogLine[] = [
  { id: 1, line: 'server started', stream: 'stdout' },
  { id: 2, line: 'ERROR db timeout', stream: 'stderr' },
  { id: 3, line: '\u001b[31mError\u001b[0m again', stream: 'stdout' },
]

describe('filterLogLines', () => {
  it.each([
    {
      name: 'no filter returns all',
      f: { text: '', stderrOnly: false },
      ids: [1, 2, 3],
    },
    {
      name: 'text is case-insensitive and ignores ANSI',
      f: { text: 'error', stderrOnly: false },
      ids: [2, 3],
    },
    { name: 'stderr only', f: { text: '', stderrOnly: true }, ids: [2] },
    { name: 'both combine', f: { text: 'db', stderrOnly: true }, ids: [2] },
  ])('$name', ({ f, ids }) => {
    expect(filterLogLines(lines, f).map((l) => l.id)).toEqual(ids)
  })
})

const leveled: LogLine[] = [
  { id: 1, line: '{"level":"info","msg":"a"}', stream: 'stdout' },
  { id: 2, line: 'WARN slow', stream: 'stdout' },
  { id: 3, line: 'level=ERROR msg=x', stream: 'stdout' },
  { id: 4, line: 'DEBUG trace', stream: 'stdout' },
  { id: 5, line: 'plain stderr', stream: 'stderr' },
]

describe('level filters', () => {
  it.each([
    { level: 'all', ids: [1, 2, 3, 4, 5] },
    { level: 'errors', ids: [3, 5] },
    { level: 'warnings', ids: [2] },
    { level: 'info', ids: [1] },
    { level: 'debug', ids: [4] },
  ] as const)('$level', ({ level, ids }) => {
    const f = { text: '', stderrOnly: false, level }
    expect(filterLogLines(leveled, f).map((l) => l.id)).toEqual(ids)
  })

  it('finds the first error index', () => {
    expect(findFirstErrorIndex(leveled)).toBe(2)
    expect(findFirstErrorIndex(leveled.slice(0, 2))).toBe(-1)
  })
})

describe('logLinesToText', () => {
  it('strips ANSI and joins with newlines', () => {
    expect(logLinesToText(lines)).toBe(
      'server started\nERROR db timeout\nError again',
    )
  })
})
