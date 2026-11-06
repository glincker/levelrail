import { describe, expect, it } from 'vitest'
import type { JanitorPlanReport } from '../../queries/janitorPlan'
import {
  buildPlan,
  defaultSelection,
  selectionTotals,
  stageSelected,
  toAllowlist,
  toggleRow,
  toggleStage,
} from './cleanupPlan'

const report: JanitorPlanReport = {
  id: 'r1',
  started_at: '2026-11-05T10:00:00Z',
  trigger: 'manual',
  dry_run: true,
  free_percent: 40,
  pressure: false,
  reclaimed_bytes: 0,
  stages: [
    {
      stage: 'api_tokens',
      status: 'ran',
      reclaimed_bytes: 0,
      items: [
        { name: 'revoked tokens', size_bytes: 0, action: 'would_remove' },
      ],
    },
    {
      stage: 'volumes',
      status: 'ran',
      reclaimed_bytes: 0,
      items: [
        { name: 'cache-vol', size_bytes: 100, action: 'would_quarantine' },
        { name: 'db-main', size_bytes: 900, action: 'kept' },
        { name: 'db-old', size_bytes: 50, action: 'would_remove' },
      ],
    },
    {
      stage: 'containers',
      status: 'ran',
      reclaimed_bytes: 0,
      items: [
        { name: 'stopped containers', size_bytes: 10, action: 'would_remove' },
      ],
    },
    { stage: 'images', status: 'ran', reclaimed_bytes: 0, items: [] },
    {
      stage: 'build_cache',
      status: 'skipped',
      reason: 'not under pressure',
      reclaimed_bytes: 0,
      items: [],
    },
  ],
}

describe('cleanupPlan', () => {
  it('orders stages, drops empty ones and keeps skipped notes', () => {
    const groups = buildPlan(report)
    expect(groups.map((g) => g.stage)).toEqual([
      'containers',
      'build_cache',
      'volumes',
      'api_tokens',
    ])
    expect(groups[1]?.note).toBe('not under pressure')
    expect(groups.filter((g) => g.whole).map((g) => g.stage)).toEqual([
      'containers',
      'build_cache',
      'api_tokens',
    ])
  })

  it('locks database volumes and kept items, selecting the rest', () => {
    const volumes = buildPlan(report).find((g) => g.stage === 'volumes')
    expect(volumes?.rows.map((r) => [r.name, r.lock])).toEqual([
      ['cache-vol', undefined],
      ['db-main', 'database'],
      ['db-old', 'database'],
    ])
    const groups = buildPlan(report)
    expect([...defaultSelection(groups)].sort()).toEqual([
      'api_tokens:revoked tokens',
      'containers:stopped containers',
      'volumes:cache-vol',
    ])
  })

  it('totals and builds the allowlist from the selection', () => {
    const groups = buildPlan(report)
    let sel = defaultSelection(groups)
    expect(selectionTotals(groups, sel)).toEqual({ count: 3, bytes: 110 })
    sel = toggleRow(sel, 'volumes', 'cache-vol')
    expect(selectionTotals(groups, sel).bytes).toBe(10)
    expect(toAllowlist(groups, sel)).toEqual([
      { stage: 'containers', names: ['stopped containers'] },
      { stage: 'api_tokens', names: ['revoked tokens'] },
    ])
  })

  it('toggles a whole stage on and off', () => {
    const groups = buildPlan(report)
    const containers = groups.find((g) => g.stage === 'containers')
    if (!containers) throw new Error('containers group missing')
    const on = defaultSelection(groups)
    expect(stageSelected(containers, on)).toBe(true)
    const off = toggleStage(on, containers)
    expect(stageSelected(containers, off)).toBe(false)
    expect(stageSelected(containers, toggleStage(off, containers))).toBe(true)
  })
})
