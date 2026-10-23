import { describe, expect, it } from 'vitest'
import {
  applyMessage,
  barWidthClass,
  EMPTY_EVENT_STATE,
  formatMs,
  groupRows,
  parseEventMessage,
  statusTone,
  verdictTone,
} from './deployEvents'
import type { DeployEvent, DeployEventTimeline } from '../types/deployEvents'

function ev(
  id: string,
  at: string,
  over: Partial<DeployEvent> = {},
): DeployEvent {
  return {
    id,
    at,
    kind: 'step',
    phase: 'deploy',
    status: 'ok',
    code: `step.${id}`,
    title: id,
    ...over,
  }
}

const timeline: DeployEventTimeline = {
  deploy_id: 'dep_1',
  app: 'web',
  verdict: 'running',
  live: true,
  frozen: false,
  started_at: '2026-10-01T12:00:00Z',
  trigger: { source: 'git_push' },
  events: [ev('b', '2026-10-01T12:00:02Z'), ev('a', '2026-10-01T12:00:01Z')],
  phases: [],
}

describe('applyMessage', () => {
  it('sorts the snapshot by time', () => {
    const s = applyMessage(EMPTY_EVENT_STATE, { type: 'snapshot', timeline })
    expect(s.events.map((e) => e.id)).toEqual(['a', 'b'])
    expect(s.deployId).toBe('dep_1')
    expect(s.live).toBe(true)
  })

  it('upserts an event by id instead of duplicating it', () => {
    let s = applyMessage(EMPTY_EVENT_STATE, { type: 'snapshot', timeline })
    s = applyMessage(s, {
      type: 'event',
      live: true,
      event: ev('a', '2026-10-01T12:00:01Z', { status: 'failed' }),
    })
    expect(s.events).toHaveLength(2)
    expect(s.events[0]?.status).toBe('failed')
  })

  it('slots a late event into time order and keeps the impact', () => {
    let s = applyMessage(EMPTY_EVENT_STATE, { type: 'snapshot', timeline })
    const impact = {
      cutover_at: '2026-10-01T12:00:01Z',
      window_ms: 1000,
      complete: true,
      before: {
        has_traffic: true,
        requests: 1,
        rate_per_sec: 1,
        error_rate_5xx: 0,
        p95_ms: 1,
      },
      after: {
        has_traffic: false,
        requests: 0,
        rate_per_sec: 0,
        error_rate_5xx: 0,
        p95_ms: 0,
      },
    }
    s = applyMessage(s, {
      type: 'event',
      live: true,
      event: ev('c', '2026-10-01T12:00:01.5Z', {
        kind: 'traffic_impact',
        impact,
      }),
    })
    expect(s.events.map((e) => e.id)).toEqual(['a', 'c', 'b'])
    expect(s.impact?.complete).toBe(true)
  })

  it('ends the stream with the final verdict', () => {
    let s = applyMessage(EMPTY_EVENT_STATE, { type: 'snapshot', timeline })
    s = applyMessage(s, { type: 'end', verdict: 'rolled_back', frozen: true })
    expect(s).toMatchObject({
      ended: true,
      live: false,
      frozen: true,
      verdict: 'rolled_back',
    })
  })

  it('state messages move the verdict and phases', () => {
    let s = applyMessage(EMPTY_EVENT_STATE, { type: 'snapshot', timeline })
    s = applyMessage(s, {
      type: 'state',
      verdict: 'watching',
      live: true,
      phases: [
        { phase: 'build', started_at: '', ended_at: '', duration_ms: 5 },
      ],
    })
    expect(s.verdict).toBe('watching')
    expect(s.phases).toHaveLength(1)
  })
})

describe('parseEventMessage', () => {
  it('accepts known types and rejects the rest', () => {
    expect(parseEventMessage('{"type":"end"}')?.type).toBe('end')
    expect(parseEventMessage('{"type":"nope"}')).toBeNull()
    expect(parseEventMessage('not json')).toBeNull()
  })
})

describe('groupRows', () => {
  it('folds consecutive log lines into one group', () => {
    const log = (id: string, at: string) =>
      ev(id, at, { kind: 'log', detail: id })
    const rows = groupRows([
      ev('t', '2026-10-01T12:00:00Z', { kind: 'trigger' }),
      log('l1', '2026-10-01T12:00:01Z'),
      log('l2', '2026-10-01T12:00:02Z'),
      ev('p', '2026-10-01T12:00:03Z'),
      log('l3', '2026-10-01T12:00:04Z'),
    ])
    expect(rows.map((r) => r.kind)).toEqual(['event', 'logs', 'event', 'logs'])
    const first = rows[1]
    expect(first?.kind === 'logs' && first.lines.length).toBe(2)
  })
})

describe('formatting', () => {
  it('formats durations', () => {
    expect(formatMs(450)).toBe('450ms')
    expect(formatMs(2500)).toBe('2.5s')
    expect(formatMs(95_000)).toBe('1m 35s')
    expect(formatMs(120_000)).toBe('2m')
  })

  it('quantizes bar widths to literal classes', () => {
    expect(barWidthClass(0, 10)).toBe('w-0')
    expect(barWidthClass(10, 10)).toBe('w-full')
    expect(barWidthClass(5, 10)).toBe('w-[50%]')
    expect(barWidthClass(1, 1000)).toBe('w-[5%]')
    expect(barWidthClass(5, 0)).toBe('w-0')
  })

  it('maps statuses and verdicts to tones', () => {
    expect(statusTone('failed')).toBe('danger')
    expect(statusTone('ok')).toBe('success')
    expect(verdictTone('rolled_back')).toBe('warning')
    expect(verdictTone('watching')).toBe('info')
  })
})
