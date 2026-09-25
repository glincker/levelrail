import { describe, expect, it } from 'vitest'
import { baseJobName, formatDuration } from './pipelineStatus'

describe('baseJobName', () => {
  it('strips the matrix suffix', () => {
    expect(baseJobName('test[go=1.22]')).toBe('test')
    expect(baseJobName('plain')).toBe('plain')
  })
})

describe('formatDuration', () => {
  it('formats seconds, minutes, and hours', () => {
    const t0 = '2026-01-01T00:00:00Z'
    expect(formatDuration(t0, '2026-01-01T00:00:42Z')).toBe('42s')
    expect(formatDuration(t0, '2026-01-01T00:03:05Z')).toBe('3m 5s')
    expect(formatDuration(t0, '2026-01-01T01:02:00Z')).toBe('1h 2m')
    expect(formatDuration(undefined)).toBe('')
  })
})
