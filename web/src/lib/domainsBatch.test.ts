import { describe, expect, it } from 'vitest'
import {
  chunk,
  itemsNeedingConfirmation,
  mergeResults,
  moveItem,
  resolveSelection,
  runChunked,
  selectAllState,
  setKeys,
  summarize,
  toggleKey,
  type BatchItem,
} from './domainsBatch'

const items = (n: number): BatchItem[] =>
  Array.from({ length: n }, (_, i) => ({ domain: `d${i}.com` }))

describe('chunk', () => {
  it('splits by size and keeps order', () => {
    const parts = chunk(items(60), 25)
    expect(parts.map((p) => p.length)).toEqual([25, 25, 10])
    expect(parts[2]?.[0]?.domain).toBe('d50.com')
  })
  it('returns nothing for an empty list', () => {
    expect(chunk([], 25)).toEqual([])
  })
})

describe('selection', () => {
  it('toggles and sets without mutating', () => {
    const a = new Set(['x'])
    const b = toggleKey(a, 'y')
    expect([...a]).toEqual(['x'])
    expect([...setKeys(b, ['x', 'z'], false)]).toEqual(['y'])
  })
  it('reports none, some and all', () => {
    expect(selectAllState(new Set(), ['a', 'b'])).toBe('none')
    expect(selectAllState(new Set(['a', 'q']), ['a', 'b'])).toBe('some')
    expect(selectAllState(new Set(['a', 'b']), ['a', 'b'])).toBe('all')
    expect(selectAllState(new Set(['a']), [])).toBe('none')
  })
  it('keeps hidden selections and drops gone or unselectable rows', () => {
    const rows = [{ d: 'a' }, { d: 'b' }, { d: 'c' }]
    const got = resolveSelection(
      new Set(['a', 'b', 'gone']),
      rows,
      (r) => r.d,
      (r) => r.d !== 'b',
    )
    expect(got).toEqual([{ d: 'a' }])
  })
})

describe('results', () => {
  it('summarizes states', () => {
    expect(
      summarize([
        { domain: 'a', ok: true },
        { domain: 'b', ok: false, error: 'x' },
        { domain: 'c', ok: false, requires_confirmation: true },
      ]),
    ).toEqual({ total: 3, succeeded: 1, failed: 1, requires_confirmation: 1 })
  })
  it('replaces a resent result in place', () => {
    const merged = mergeResults(
      [
        { domain: 'a', ok: true },
        { domain: 'b', ok: false, requires_confirmation: true },
      ],
      [{ domain: 'b', ok: true }],
    )
    expect(merged).toEqual([
      { domain: 'a', ok: true },
      { domain: 'b', ok: true },
    ])
  })
  it('picks only items waiting on confirmation', () => {
    const waiting = itemsNeedingConfirmation(items(3), [
      { domain: 'd0.com', ok: true },
      { domain: 'd1.com', requires_confirmation: true },
      { domain: 'd2.com', error: 'no' },
    ])
    expect(waiting.map((i) => i.domain)).toEqual(['d1.com'])
  })
})

describe('runChunked', () => {
  it('merges chunk results and sends the same action to each', async () => {
    const seen: number[] = []
    const out = await runChunked(
      { action: 'park', items: items(30) },
      (req) => {
        seen.push(req.items.length)
        return Promise.resolve({
          results: req.items.map((i) => ({ domain: i.domain, ok: true })),
        })
      },
      25,
    )
    expect(seen).toEqual([25, 5])
    expect(out).toHaveLength(30)
  })
  it('fails only the items of a rejected chunk', async () => {
    let n = 0
    const out = await runChunked(
      { action: 'park', items: items(4) },
      (req) => {
        n++
        if (n === 2) return Promise.reject(new Error('too many'))
        return Promise.resolve({
          results: req.items.map((i) => ({ domain: i.domain, ok: true })),
        })
      },
      2,
    )
    expect(out.map((r) => r.ok)).toEqual([true, true, false, false])
    expect(out[3]?.error).toBe('too many')
  })
  it('marks unanswered items failed', async () => {
    const out = await runChunked({ action: 'park', items: items(2) }, () =>
      Promise.resolve({ results: [{ domain: 'd0.com', ok: true }] }),
    )
    expect(out[1]?.ok).toBe(false)
  })
})

describe('moveItem', () => {
  it('names the kind only for static sites', () => {
    const d = { domain: 'a.com', service_name: 'web', environment_id: 'e1' }
    expect(moveItem(d, { name: 'api', kind: 'app' }, 'e2')).toEqual({
      domain: 'a.com',
      from: 'web',
      to: 'api',
      from_environment: 'e1',
      to_environment: 'e2',
    })
    expect(moveItem(d, { name: 'docs', kind: 'static_site' }, 'e2')).toEqual({
      domain: 'a.com',
      from: 'web',
      to: 'docs',
      from_environment: 'e1',
      to_kind: 'static_site',
    })
  })
})
