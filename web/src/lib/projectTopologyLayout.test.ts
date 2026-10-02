import { describe, expect, it } from 'vitest'
import { groupIntoColumns } from './projectTopologyLayout'
import type { ProjectTopologyGraph } from '../types/projectTopology'

describe('groupIntoColumns', () => {
  it('buckets nodes by kind in app, database, volume order', () => {
    const graph: ProjectTopologyGraph = {
      nodes: [
        {
          id: 'volume:app-web-uploads',
          kind: 'volume',
          label: 'app-web-uploads',
        },
        { id: 'database:main', kind: 'database', label: 'main' },
        { id: 'app:web', kind: 'app', label: 'web' },
        { id: 'app:worker', kind: 'app', label: 'worker' },
      ],
      edges: [],
    }
    const columns = groupIntoColumns(graph)
    expect(columns.map((c) => c.kind)).toEqual(['app', 'database', 'volume'])
    expect(columns[0]?.nodes.map((n) => n.label)).toEqual(['web', 'worker'])
    expect(columns[1]?.nodes.map((n) => n.label)).toEqual(['main'])
    expect(columns[2]?.nodes.map((n) => n.label)).toEqual(['app-web-uploads'])
  })

  it('drops a column with no nodes instead of rendering it empty', () => {
    const graph: ProjectTopologyGraph = {
      nodes: [{ id: 'app:web', kind: 'app', label: 'web' }],
      edges: [],
    }
    const columns = groupIntoColumns(graph)
    expect(columns).toHaveLength(1)
    expect(columns[0]?.kind).toBe('app')
  })

  it('returns no columns for an empty graph', () => {
    expect(groupIntoColumns({ nodes: [], edges: [] })).toEqual([])
  })
})
