import type {
  ProjectTopologyEdge,
  ProjectTopologyGraph,
  ProjectTopologyNode,
} from '../types/projectTopology'
import type { IacFile } from '../queries/iac'
import type { AppConnection } from '../queries/appConnections'

export type DraftEngine = 'postgres' | 'redis' | 'mysql'

export type DraftOp =
  | { id: string; type: 'add-app'; name: string; image: string; port: number }
  | {
      id: string
      type: 'add-database'
      name: string
      engine: DraftEngine
      version: string
    }
  | {
      id: string
      type: 'connect'
      app: string
      database: string
      envVar: string
    }
  | { id: string; type: 'disconnect'; app: string; database: string }

export type ConnectionStep =
  | { kind: 'connect'; app: string; database: string; envVar: string }
  | { kind: 'disconnect'; app: string; database: string; envVar: string }

export const DRAFT_ENGINES: readonly DraftEngine[] = [
  'postgres',
  'redis',
  'mysql',
]

const NAME_RE = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/
const ENV_VAR_RE = /^[A-Za-z_][A-Za-z0-9_]*$/

export const appNodeId = (name: string) => `app:${name}`
export const databaseNodeId = (name: string) => `database:${name}`

export type NameError = 'required' | 'format' | 'taken'

export function validateNewName(
  raw: string,
  graph: ProjectTopologyGraph,
  kind: 'app' | 'database',
): NameError | null {
  const name = raw.trim()
  if (!name) return 'required'
  if (!NAME_RE.test(name)) return 'format'
  const id = kind === 'app' ? appNodeId(name) : databaseNodeId(name)
  return graph.nodes.some((n) => n.id === id) ? 'taken' : null
}

export function validateEnvVar(raw: string): boolean {
  return raw === '' || ENV_VAR_RE.test(raw)
}

export function validatePort(port: number): boolean {
  return Number.isInteger(port) && port >= 1 && port <= 65535
}

// Removing a node from the draft also drops any connection that pointed at it.
export function removeOp(ops: readonly DraftOp[], id: string): DraftOp[] {
  const target = ops.find((o) => o.id === id)
  if (!target) return [...ops]
  return ops.filter((o) => {
    if (o.id === id) return false
    if (target.type === 'add-app') return !('app' in o) || o.app !== target.name
    if (target.type === 'add-database') {
      return !('database' in o) || o.database !== target.name
    }
    return true
  })
}

export function addConnection(
  ops: readonly DraftOp[],
  graph: ProjectTopologyGraph,
  app: string,
  database: string,
  envVar: string,
  id: string,
): DraftOp[] {
  const exists = effectiveGraph(graph, ops).edges.some(
    (e) =>
      e.kind === 'database_binding' &&
      e.from === appNodeId(app) &&
      e.to === databaseNodeId(database),
  )
  if (exists) return [...ops]
  const undone = ops.find(
    (o) => o.type === 'disconnect' && o.app === app && o.database === database,
  )
  if (undone) return ops.filter((o) => o.id !== undone.id)
  return [...ops, { id, type: 'connect', app, database, envVar }]
}

// A drawn edge is either a draft connect (drop it) or a live one (queue a disconnect).
export function removeConnection(
  ops: readonly DraftOp[],
  app: string,
  database: string,
  id: string,
): DraftOp[] {
  const draft = ops.find(
    (o) => o.type === 'connect' && o.app === app && o.database === database,
  )
  if (draft) return ops.filter((o) => o.id !== draft.id)
  if (
    ops.some(
      (o) =>
        o.type === 'disconnect' && o.app === app && o.database === database,
    )
  ) {
    return [...ops]
  }
  return [...ops, { id, type: 'disconnect', app, database }]
}

// The graph the canvas draws: live topology plus the draft, new nodes marked Draft.
export function effectiveGraph(
  graph: ProjectTopologyGraph,
  ops: readonly DraftOp[],
): ProjectTopologyGraph {
  if (ops.length === 0) return graph
  const nodes: ProjectTopologyNode[] = [...graph.nodes]
  let edges: ProjectTopologyEdge[] = [...graph.edges]
  for (const op of ops) {
    if (op.type === 'add-app') {
      nodes.push({
        id: appNodeId(op.name),
        kind: 'app',
        label: op.name,
        draft: true,
      })
    } else if (op.type === 'add-database') {
      nodes.push({
        id: databaseNodeId(op.name),
        kind: 'database',
        label: op.name,
        draft: true,
      })
    } else if (op.type === 'connect') {
      edges.push({
        from: appNodeId(op.app),
        to: databaseNodeId(op.database),
        kind: 'database_binding',
      })
    } else {
      edges = edges.filter(
        (e) =>
          !(
            e.kind === 'database_binding' &&
            e.from === appNodeId(op.app) &&
            e.to === databaseNodeId(op.database)
          ),
      )
    }
  }
  return { nodes, edges }
}

// Files for POST /api/v1/apply/plan. JSON is valid YAML, so values are escaped by JSON.stringify.
export function draftFiles(
  ops: readonly DraftOp[],
  projectName: string,
): IacFile[] {
  const files: IacFile[] = []
  for (const op of ops) {
    if (op.type === 'add-app') {
      files.push({
        name: `canvas-app-${op.name}.yaml`,
        content: JSON.stringify(
          {
            version: 1,
            kind: 'App',
            metadata: { name: op.name },
            spec: {
              project: projectName,
              service: {
                build: { type: 'image', image: op.image },
                port: op.port,
              },
            },
          },
          null,
          2,
        ),
      })
    } else if (op.type === 'add-database') {
      const spec: Record<string, string> = {
        engine: op.engine,
        project: projectName,
      }
      if (op.version.trim()) spec.version = op.version.trim()
      files.push({
        name: `canvas-database-${op.name}.yaml`,
        content: JSON.stringify(
          { version: 1, kind: 'Database', metadata: { name: op.name }, spec },
          null,
          2,
        ),
      })
    }
  }
  return files
}

export interface ResolvedSteps {
  steps: ConnectionStep[]
  // Edges the connections API does not own (declared in app.yaml), left untouched.
  unsupported: { app: string; database: string }[]
}

export function resolveConnectionSteps(
  ops: readonly DraftOp[],
  live: Readonly<Record<string, readonly AppConnection[]>>,
): ResolvedSteps {
  const steps: ConnectionStep[] = []
  const unsupported: { app: string; database: string }[] = []
  for (const op of ops) {
    if (op.type === 'connect') {
      steps.push({
        kind: 'connect',
        app: op.app,
        database: op.database,
        envVar: op.envVar,
      })
    } else if (op.type === 'disconnect') {
      const matches = (live[op.app] ?? []).filter(
        (c) => c.database_name === op.database,
      )
      if (matches.length === 0) {
        unsupported.push({ app: op.app, database: op.database })
      }
      for (const c of matches) {
        steps.push({
          kind: 'disconnect',
          app: op.app,
          database: op.database,
          envVar: c.env_var,
        })
      }
    }
  }
  return { steps, unsupported }
}

export const hasPlanOps = (ops: readonly DraftOp[]) =>
  ops.some((o) => o.type === 'add-app' || o.type === 'add-database')

export const disconnectApps = (ops: readonly DraftOp[]): string[] =>
  Array.from(
    new Set(ops.flatMap((o) => (o.type === 'disconnect' ? [o.app] : []))),
  )
