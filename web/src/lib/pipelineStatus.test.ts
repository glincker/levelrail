import { describe, expect, it } from 'vitest'
import { baseJobName, formatDuration, stepLink } from './pipelineStatus'

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

describe('stepLink', () => {
  const page = 'https://x.test/apps/a/pipelines/runs/r1?job=old&step=9#top'
  it.each([
    [undefined, 'https://x.test/apps/a/pipelines/runs/r1?job=test%5Bgo%3D1%5D'],
    [2, 'https://x.test/apps/a/pipelines/runs/r1?job=test%5Bgo%3D1%5D&step=2'],
  ])('step %s replaces the old selection', (step, want) => {
    expect(stepLink(page, 'test[go=1]', step)).toBe(want)
  })
})
