import { describe, expect, it } from 'vitest'
import { parsePlan, type PlanNode } from './plan'

const flat = (n: PlanNode[]): PlanNode[] =>
  n.flatMap((x) => [x, ...flat(x.children)])

describe('parsePlan', () => {
  it('parses a lone seq scan with its filter', () => {
    const tree = parsePlan([
      'Seq Scan on users  (cost=0.00..155.00 rows=10 width=64)',
      '  Filter: (age > 30)',
    ])
    expect(tree).toHaveLength(1)
    expect(tree?.[0]).toMatchObject({
      label: 'Seq Scan on users',
      totalCost: 155,
      rows: 10,
      hottest: true,
      details: ['Filter: (age > 30)'],
    })
  })

  it('nests a loop over an index scan and flags the costliest own node', () => {
    const tree = parsePlan([
      'Nested Loop  (cost=0.29..420.50 rows=5 width=40) (actual time=0.05..2.10 rows=5 loops=1)',
      '  ->  Seq Scan on orders o  (cost=0.00..400.00 rows=100 width=20)',
      '        Filter: (total > 10)',
      '  ->  Index Scan using users_pkey on users u  (cost=0.29..0.31 rows=1 width=20)',
      '        Index Cond: (id = o.user_id)',
    ])
    const root = tree?.[0]
    expect(root?.children.map((c) => c.label)).toEqual([
      'Seq Scan on orders o',
      'Index Scan using users_pkey on users u',
    ])
    expect(root?.actualMs).toBe(2.1)
    expect(
      flat(tree ?? [])
        .filter((n) => n.hottest)
        .map((n) => n.label),
    ).toEqual(['Seq Scan on orders o'])
  })

  it('handles hash join with a sort and deeper nesting', () => {
    const tree = parsePlan([
      'Sort  (cost=300.00..302.00 rows=800 width=40)',
      '  Sort Key: u.email',
      '  ->  Hash Join  (cost=50.00..250.00 rows=800 width=40)',
      '        Hash Cond: (o.user_id = u.id)',
      '        ->  Seq Scan on orders o  (cost=0.00..120.00 rows=5000 width=20)',
      '        ->  Hash  (cost=30.00..30.00 rows=1000 width=20)',
      '              ->  Seq Scan on users u  (cost=0.00..30.00 rows=1000 width=20)',
    ])
    const sort = tree?.[0]
    const join = sort?.children[0]
    expect(sort?.details).toEqual(['Sort Key: u.email'])
    expect(join?.children.map((c) => c.label)).toEqual([
      'Seq Scan on orders o',
      'Hash',
    ])
    expect(join?.children[1]?.children[0]?.label).toBe('Seq Scan on users u')
    expect(flat(tree ?? []).filter((n) => n.hottest)).toHaveLength(1)
  })

  it('returns null when no line has a cost', () => {
    expect(parsePlan(['Result', '  One-Time Filter: false'])).toBeNull()
  })
})
