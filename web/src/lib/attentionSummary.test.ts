import { describe, expect, it } from 'vitest'
import type { AttentionItem } from './attention'
import { attentionHref, summarizeAttention } from './attentionSummary'

const item = (
  id: string,
  severity: AttentionItem['severity'],
): AttentionItem => ({
  id,
  severity,
  title: id,
  detail: '',
  target: { kind: 'system' },
})

const none = () => false

describe('summarizeAttention', () => {
  it.each([
    ['empty', [], 'ok', 0],
    ['info only is not an issue', [item('u', 'info')], 'ok', 0],
    ['warnings', [item('a', 'warning'), item('b', 'warning')], 'warning', 2],
    [
      'any critical wins and all actionable items count',
      [item('a', 'warning'), item('b', 'critical'), item('c', 'warning')],
      'critical',
      3,
    ],
  ] as const)('%s', (_n, items, level, count) => {
    const s = summarizeAttention([...items], none)
    expect(s.level).toBe(level)
    expect(s.count).toBe(count)
  })

  it('moves snoozed items out of the count and level', () => {
    const s = summarizeAttention(
      [item('a', 'critical'), item('b', 'warning')],
      (id) => id === 'a',
    )
    expect(s.count).toBe(1)
    expect(s.level).toBe('warning')
    expect(s.snoozed.map((i) => i.id)).toEqual(['a'])
  })
})

describe('attentionHref', () => {
  it('maps targets to routes', () => {
    expect(attentionHref({ kind: 'app', name: 'a b' })).toBe(
      '/apps/a%20b/overview',
    )
    expect(attentionHref({ kind: 'node', id: 'n1' })).toBe('/nodes/n1')
    expect(attentionHref({ kind: 'system' })).toBe('/status')
  })
})
