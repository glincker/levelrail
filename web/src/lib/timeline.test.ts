import { describe, expect, it } from 'vitest'
import { sortTimeline } from './timeline'
import type { TimelineEvent } from '../types/investigate'

function ev(
  at: string,
  title: string,
  severity: TimelineEvent['severity'] = 'info',
): TimelineEvent {
  return { at, kind: 'deploy', severity, title }
}

describe('sortTimeline', () => {
  it('orders oldest first', () => {
    const out = sortTimeline([
      ev('2026-10-10T03:05:00Z', 'restart'),
      ev('2026-10-10T03:00:00Z', 'deploy'),
      ev('2026-10-10T03:10:00Z', 'alert'),
    ])
    expect(out.map((e) => e.title)).toEqual(['deploy', 'restart', 'alert'])
  })

  it('puts the more severe event first at the same instant, then input order', () => {
    const out = sortTimeline([
      ev('2026-10-10T03:00:00Z', 'info a'),
      ev('2026-10-10T03:00:00Z', 'critical', 'critical'),
      ev('2026-10-10T03:00:00Z', 'info b'),
      ev('2026-10-10T03:00:00Z', 'warning', 'warning'),
    ])
    expect(out.map((e) => e.title)).toEqual([
      'critical',
      'warning',
      'info a',
      'info b',
    ])
  })

  it('does not mutate its input', () => {
    const input = [
      ev('2026-10-10T04:00:00Z', 'b'),
      ev('2026-10-10T03:00:00Z', 'a'),
    ]
    sortTimeline(input)
    expect(input[0]?.title).toBe('b')
  })
})
