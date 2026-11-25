// Goal-first entry for the create modal: each goal maps to template
// catalog categories (service template `category` values) and to the
// wizard path that serves it. Data only, no UI.

export type GoalId =
  | 'website'
  | 'api'
  | 'database'
  | 'static-site'
  | 'scheduled-job'
  | 'self-hosted'

// app: prefilled image form, git: build from a repo, database: engine
// picker, templates: catalog narrowed to the goal's categories.
export type GoalKind = 'app' | 'git' | 'database' | 'templates'

export interface GoalDef {
  id: GoalId
  kind: GoalKind
  /** Service template categories this goal draws from. */
  categories: readonly string[]
  /** Prefill for kind "app": suggested port and health path (null = off). */
  defaults?: { port: number; healthPath: string | null }
}

export const GOALS: readonly GoalDef[] = [
  {
    id: 'website',
    kind: 'app',
    categories: ['Applications'],
    defaults: { port: 8080, healthPath: '/' },
  },
  {
    id: 'api',
    kind: 'app',
    categories: ['Developer Tools'],
    defaults: { port: 8080, healthPath: '/healthz' },
  },
  {
    id: 'database',
    kind: 'database',
    categories: ['Databases', 'Database Tools'],
  },
  { id: 'static-site', kind: 'git', categories: ['Applications'] },
  {
    id: 'scheduled-job',
    kind: 'templates',
    categories: ['Backups', 'Developer Tools'],
  },
  {
    id: 'self-hosted',
    kind: 'templates',
    categories: [
      'Applications',
      'Productivity',
      'Media',
      'Communication',
      'Storage',
      'Monitoring',
    ],
  },
]

export function goalById(id: string): GoalDef | undefined {
  return GOALS.find((g) => g.id === id)
}

/** Goals that apply to a wizard scope; a database-only entry hides app goals. */
export function goalsForScope(
  scope: 'applications' | 'databases' | undefined,
): readonly GoalDef[] {
  if (scope === 'databases') return GOALS.filter((g) => g.kind === 'database')
  if (scope === 'applications')
    return GOALS.filter((g) => g.kind !== 'database')
  return GOALS
}
