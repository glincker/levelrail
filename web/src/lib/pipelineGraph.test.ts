import { describe, expect, it } from 'vitest'
import type { PipelineJob, PipelineStatus } from '../types/pipelines'
import {
  NODE_WIDTH,
  assignDepths,
  layoutGraph,
  neighborKey,
  rollupStatus,
} from './pipelineGraph'

function job(
  key: string,
  needs: string[] = [],
  status: PipelineStatus = 'pending',
): PipelineJob {
  return { key, name: key, needs, status, attempt: 0, steps: [] }
}

function columnOf(jobs: PipelineJob[], id: string): number | undefined {
  return layoutGraph(jobs).nodes.find((n) => n.id === id)?.column
}

describe('assignDepths', () => {
  it.each([
    ['no needs', { a: [] as string[] }, { a: 0 }],
    ['chain', { a: [], b: ['a'], c: ['b'] }, { a: 0, b: 1, c: 2 }],
    [
      'deepest parent wins',
      { a: [], b: ['a'], c: ['a', 'b'] },
      { a: 0, b: 1, c: 2 },
    ],
    ['unknown need is ignored', { a: ['ghost'] }, { a: 0 }],
    ['self need is ignored', { a: ['a'] }, { a: 0 }],
  ])('%s', (_name, needs, want) => {
    const got = assignDepths(new Map(Object.entries(needs)))
    expect(Object.fromEntries(got)).toEqual(want)
  })

  it('terminates on a cycle and depths every job', () => {
    const got = assignDepths(
      new Map([
        ['a', ['b']],
        ['b', ['c']],
        ['c', ['a']],
      ]),
    )
    expect([...got.keys()].sort()).toEqual(['a', 'b', 'c'])
  })
})

describe('layoutGraph', () => {
  it('lays jobs out in columns by depth with an edge per need', () => {
    const jobs = [
      job('lint'),
      job('test'),
      job('build', ['lint', 'test']),
      job('deploy', ['build']),
    ]
    const layout = layoutGraph(jobs)
    expect(columnOf(jobs, 'lint')).toBe(0)
    expect(columnOf(jobs, 'build')).toBe(1)
    expect(columnOf(jobs, 'deploy')).toBe(2)
    expect(layout.edges.map((e) => `${e.from}>${e.to}`).sort()).toEqual([
      'build>deploy',
      'lint>build',
      'test>build',
    ])
    expect(layout.edges.every((e) => e.path.startsWith('M '))).toBe(true)
  })

  it('groups matrix jobs into one node with one selectable member each', () => {
    const jobs = [
      job('test[go=1.22]', [], 'succeeded'),
      job('test[go=1.23]', [], 'failed'),
      job('ship', ['test']),
    ]
    const layout = layoutGraph(jobs)
    const group = layout.nodes.find((n) => n.id === 'test')
    expect(group?.grouped).toBe(true)
    expect(group?.members.map((m) => m.label)).toEqual(['go=1.22', 'go=1.23'])
    expect(group?.status).toBe('failed')
    expect(layout.nodes).toHaveLength(2)
    expect(layout.edges).toHaveLength(1)
  })

  it('does not group a plain job and gives it one member', () => {
    const node = layoutGraph([job('lint')]).nodes[0]
    expect(node?.grouped).toBe(false)
    expect(node?.members.map((m) => m.key)).toEqual(['lint'])
  })

  it('survives a cycle and a missing dependency', () => {
    const layout = layoutGraph([
      job('a', ['b']),
      job('b', ['a']),
      job('c', ['zzz']),
    ])
    expect(layout.nodes).toHaveLength(3)
    expect(layout.width).toBeGreaterThan(0)
  })

  it('returns an empty layout for no jobs', () => {
    expect(layoutGraph([])).toMatchObject({ nodes: [], edges: [] })
  })

  it('orders a column by where its parents sit', () => {
    const jobs = [
      job('a'),
      job('b'),
      job('child-of-b', ['b']),
      job('child-of-a', ['a']),
    ]
    const col1 = layoutGraph(jobs)
      .nodes.filter((n) => n.column === 1)
      .sort((x, y) => x.y - y.y)
      .map((n) => n.id)
    expect(col1).toEqual(['child-of-a', 'child-of-b'])
  })

  it('lays out 100 jobs without overlapping rows', () => {
    const jobs: PipelineJob[] = [job('root')]
    for (let i = 0; i < 49; i++) {
      jobs.push(job(`mid${i}`, ['root']))
    }
    for (let i = 0; i < 49; i++) {
      jobs.push(job(`leaf${i}`, [`mid${i}`]))
    }
    jobs.push(job('final', ['leaf0', 'leaf48']))
    expect(jobs).toHaveLength(100)
    const layout = layoutGraph(jobs)
    expect(layout.nodes).toHaveLength(100)
    expect(layout.edges).toHaveLength(49 + 49 + 2)
    const byColumn = new Map<number, number[]>()
    for (const n of layout.nodes) {
      byColumn.set(n.column, [...(byColumn.get(n.column) ?? []), n.y])
    }
    for (const ys of byColumn.values()) {
      expect(new Set(ys).size).toBe(ys.length)
    }
    expect(layout.width).toBeGreaterThanOrEqual(3 * NODE_WIDTH)
  })
})

describe('rollupStatus', () => {
  it.each<[PipelineStatus[], PipelineStatus]>([
    [['succeeded', 'failed'], 'failed'],
    [['succeeded', 'running'], 'running'],
    [['waiting_approval', 'pending'], 'waiting_approval'],
    [['cancelled', 'succeeded'], 'cancelled'],
    [['skipped', 'skipped'], 'skipped'],
    [['succeeded', 'skipped'], 'succeeded'],
    [['succeeded', 'pending'], 'pending'],
    [[], 'pending'],
  ])('%j -> %s', (input, want) => {
    expect(rollupStatus(input)).toBe(want)
  })
})

describe('neighborKey', () => {
  const layout = layoutGraph([
    job('a'),
    job('b'),
    job('c', ['a']),
    job('d', ['b']),
  ])

  it('moves within and across columns', () => {
    expect(neighborKey(layout, 'a', 'down')).toBe('b')
    expect(neighborKey(layout, 'a', 'up')).toBeUndefined()
    expect(neighborKey(layout, 'a', 'right')).toBe('c')
    expect(neighborKey(layout, 'd', 'left')).toBe('b')
    expect(neighborKey(layout, 'c', 'right')).toBeUndefined()
    expect(neighborKey(layout, 'nope', 'right')).toBeUndefined()
  })
})
