import { describe, expect, it } from 'vitest'
import {
  allChecks,
  countSeverity,
  groupChecks,
  reduceDoctor,
  safeAutoFixes,
  startDoctorState,
} from './doctorModel'
import type { DoctorCheck } from '../queries/systemDoctor'
import type { Fix } from '../queries/fixit'

function check(over: Partial<DoctorCheck>): DoctorCheck {
  return { code: 'c', name: 'C', status: 'ok', message: 'fine', ...over }
}

function fix(over: Partial<Fix>): Fix {
  return {
    id: 'f',
    check_id: 'c',
    subject: 's',
    title: 't',
    why: 'w',
    risk: 'safe',
    reversible: true,
    state: 'offer',
    base_kind: 'auto',
    kind: 'auto',
    failing: true,
    detail: '',
    changes: [],
    attempts: [],
    ...over,
  }
}

describe('reduceDoctor', () => {
  it('collects checks as they finish and attaches fixes at the end', () => {
    let s = startDoctorState()
    s = reduceDoctor(s, { type: 'start', total: 2 })
    s = reduceDoctor(s, {
      type: 'check',
      id: 'disk',
      checks: [check({ code: 'disk_space', status: 'warn' })],
    })
    s = reduceDoctor(s, {
      type: 'check',
      id: 'docker',
      checks: [check({ code: 'docker' })],
    })
    expect(s.finished).toBe(2)
    expect(s.running).toBe(true)
    s = reduceDoctor(s, {
      type: 'fixes',
      checks: [
        check({ code: 'disk_space', fix_id: 'disk.reclaim', fix_kind: 'auto' }),
      ],
    })
    s = reduceDoctor(s, { type: 'done', ok: true })
    expect(s.running).toBe(false)
    const disk = allChecks(s).find((c) => c.code === 'disk_space')
    expect(disk?.fix_id).toBe('disk.reclaim')
  })
})

describe('groupChecks', () => {
  it('orders groups, puts problems first and files unknown groups under other', () => {
    const groups = groupChecks([
      check({ code: 'a', name: 'A', group: 'network', status: 'ok' }),
      check({ code: 'b', name: 'B', group: 'network', status: 'fail' }),
      check({ code: 'c', name: 'C', group: 'runtime' }),
      check({ code: 'd', name: 'D', group: 'made-up' }),
      check({ code: 'e', name: 'E' }),
    ])
    expect(groups.map((g) => g.id)).toEqual(['runtime', 'network', 'other'])
    expect(groups[1]?.checks.map((c) => c.code)).toEqual(['b', 'a'])
    expect(groups[2]?.checks).toHaveLength(2)
  })
})

describe('countSeverity', () => {
  it('counts every status', () => {
    const c = countSeverity([
      check({ status: 'fail' }),
      check({ status: 'warn' }),
      check({ status: 'warn' }),
      check({ status: 'unknown' }),
      check({ status: 'ok' }),
    ])
    expect(c).toEqual({ fail: 1, warn: 2, unknown: 1, ok: 1 })
  })
})

describe('safeAutoFixes', () => {
  it('keeps only failing, one-click, safe fixes with nothing to choose', () => {
    const kept = safeAutoFixes([
      fix({ id: 'keep' }),
      fix({ id: 'guided', kind: 'guided' }),
      fix({ id: 'risky', risk: 'exposes' }),
      fix({ id: 'restarts', risk: 'restarts' }),
      fix({ id: 'healthy', failing: false }),
      fix({ id: 'limited', state: 'rate_limited' }),
      fix({ id: 'param', param: { name: 'host', options: [], free: true } }),
    ])
    expect(kept.map((f) => f.id)).toEqual(['keep'])
  })
})
