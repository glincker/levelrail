import { describe, expect, it } from 'vitest'
import type { LogLine } from '../hooks/useLogStream'
import { filterLogLines, logLinesToText } from './logFilter'

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

describe('logLinesToText', () => {
  it('strips ANSI and joins with newlines', () => {
    expect(logLinesToText(lines)).toBe(
      'server started\nERROR db timeout\nError again',
    )
  })
})
