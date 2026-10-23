import { describe, expect, it } from 'vitest'
import type { ProjectTopologyGraph } from '../types/projectTopology'
import type { AppConnection } from '../queries/appConnections'
import {
  addConnection,
  draftFiles,
  effectiveGraph,
  removeConnection,
  removeOp,
  resolveConnectionSteps,
  validateEnvVar,
  validateNewName,
  validatePort,
  type DraftOp,
} from './canvasDraft'

const graph: ProjectTopologyGraph = {
  nodes: [
    { id: 'app:web', kind: 'app', label: 'web' },
    { id: 'database:main', kind: 'database', label: 'main' },
  ],
  edges: [{ from: 'app:web', to: 'database:main', kind: 'database_binding' }],
}

const app: DraftOp = {
  id: '1',
  type: 'add-app',
  name: 'api',
  image: 'ghcr.io/acme/api:1',
  port: 8080,
}
const db: DraftOp = {
  id: '2',
  type: 'add-database',
  name: 'cache',
  engine: 'redis',
  version: ' 7 ',
}

interface ParsedDoc {
  spec: { service?: { build: { image: string } } } & Record<string, unknown>
}
const parse = (content: string) => JSON.parse(content) as ParsedDoc

describe('draftFiles', () => {
  it('maps adds to App and Database documents scoped to the project', () => {
    const files = draftFiles([app, db], 'shop')
    expect(files.map((f) => f.name)).toEqual([
      'canvas-app-api.yaml',
      'canvas-database-cache.yaml',
    ])
    expect(JSON.parse(files[0]?.content ?? '')).toEqual({
      version: 1,
      kind: 'App',
      metadata: { name: 'api' },
      spec: {
        project: 'shop',
        service: {
          build: { type: 'image', image: 'ghcr.io/acme/api:1' },
          port: 8080,
        },
      },
    })
    expect(parse(files[1]?.content ?? '').spec).toEqual({
      engine: 'redis',
      project: 'shop',
      version: '7',
    })
  })

  it('leaves connection edits out of the plan files', () => {
    const connect: DraftOp = {
      id: '3',
      type: 'connect',
      app: 'web',
      database: 'main',
      envVar: '',
    }
    expect(draftFiles([connect], 'shop')).toEqual([])
  })

  it('escapes hostile values instead of injecting YAML', () => {
    const evil: DraftOp = { ...app, image: 'x\n---\nkind: Project' }
    const file = draftFiles([evil], 'shop')[0] ?? { name: '', content: '' }
    expect(parse(file.content).spec.service?.build.image).toBe(
      'x\n---\nkind: Project',
    )
    expect(file.content.split('\n').filter((l) => l === '---')).toEqual([])
  })
})

describe('effectiveGraph', () => {
  it('returns the live graph untouched for an empty draft', () => {
    expect(effectiveGraph(graph, [])).toBe(graph)
  })

  it('adds draft nodes flagged as draft and wires connections', () => {
    const out = effectiveGraph(graph, [
      app,
      { id: '3', type: 'connect', app: 'api', database: 'main', envVar: '' },
    ])
    expect(out.nodes.find((n) => n.id === 'app:api')?.draft).toBe(true)
    expect(out.edges).toContainEqual({
      from: 'app:api',
      to: 'database:main',
      kind: 'database_binding',
    })
    expect(graph.nodes).toHaveLength(2)
  })

  it('hides a live edge once a disconnect is queued', () => {
    const out = effectiveGraph(graph, [
      { id: '4', type: 'disconnect', app: 'web', database: 'main' },
    ])
    expect(out.edges).toEqual([])
  })
})

describe('draft edits', () => {
  it('does not duplicate an existing connection', () => {
    expect(addConnection([], graph, 'web', 'main', '', 'x')).toEqual([])
  })

  it('re-adding a queued disconnect cancels it', () => {
    const queued = removeConnection([], 'web', 'main', 'd1')
    expect(queued).toHaveLength(1)
    expect(addConnection(queued, graph, 'web', 'main', '', 'x')).toEqual([])
  })

  it('removing a draft connection drops it, removing a live one queues a disconnect', () => {
    const withDraft = addConnection([app], graph, 'api', 'main', 'DB', 'c1')
    expect(removeConnection(withDraft, 'api', 'main', 'z')).toEqual([app])
    expect(removeConnection([], 'web', 'main', 'z')).toEqual([
      { id: 'z', type: 'disconnect', app: 'web', database: 'main' },
    ])
  })

  it('removing a draft node also drops connections that used it', () => {
    const ops = addConnection([app, db], graph, 'api', 'cache', '', 'c1')
    expect(ops).toHaveLength(3)
    expect(removeOp(ops, '1').map((o) => o.id)).toEqual(['2'])
    expect(removeOp(ops, '2').map((o) => o.id)).toEqual(['1'])
  })
})

describe('validation', () => {
  it.each([
    ['', 'required'],
    ['Web', 'format'],
    ['-x', 'format'],
    ['web', 'taken'],
    ['api', null],
  ] as const)('app name %j -> %s', (name, want) => {
    expect(validateNewName(name, graph, 'app')).toBe(want)
  })

  it('treats app and database names as separate namespaces', () => {
    expect(validateNewName('main', graph, 'app')).toBeNull()
    expect(validateNewName('main', graph, 'database')).toBe('taken')
  })

  it('validates ports and env var names', () => {
    expect(validatePort(0)).toBe(false)
    expect(validatePort(65536)).toBe(false)
    expect(validatePort(80)).toBe(true)
    expect(validateEnvVar('')).toBe(true)
    expect(validateEnvVar('DATABASE_URL')).toBe(true)
    expect(validateEnvVar('1BAD')).toBe(false)
  })
})

describe('resolveConnectionSteps', () => {
  const live: Record<string, AppConnection[]> = {
    web: [
      {
        env_var: 'DATABASE_URL',
        database_name: 'main',
        field: 'url',
        host: 'h',
        mesh_dns: false,
        cross_node: false,
      },
    ],
  }

  it('turns connects into steps and finds the env var to disconnect', () => {
    const { steps, unsupported } = resolveConnectionSteps(
      [
        { id: '1', type: 'connect', app: 'api', database: 'main', envVar: '' },
        { id: '2', type: 'disconnect', app: 'web', database: 'main' },
      ],
      live,
    )
    expect(steps).toEqual([
      { kind: 'connect', app: 'api', database: 'main', envVar: '' },
      {
        kind: 'disconnect',
        app: 'web',
        database: 'main',
        envVar: 'DATABASE_URL',
      },
    ])
    expect(unsupported).toEqual([])
  })

  it('reports a binding the connections API does not own', () => {
    const { steps, unsupported } = resolveConnectionSteps(
      [{ id: '1', type: 'disconnect', app: 'worker', database: 'main' }],
      live,
    )
    expect(steps).toEqual([])
    expect(unsupported).toEqual([{ app: 'worker', database: 'main' }])
  })
})
