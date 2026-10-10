// Draft model of the guided policy builder and its conversion to and from the
// stored document. The document format is unchanged: a draft only compiles to
// Effect, Action and Resource lists the evaluator already understands.

import type { PolicyDocument } from '../queries/iamPolicies'

export type Effect = 'Allow' | 'Deny'

export type ResourceSelection =
  | { kind: 'all' }
  | { kind: 'app'; name: string }
  | { kind: 'database'; name: string }
  | { kind: 'environment'; id: string }
  | { kind: 'environment_kind'; value: string }
  | { kind: 'project'; id: string; environmentIds: string[] }
  | { kind: 'pattern'; value: string }

export interface DraftStatement {
  effect: Effect
  actions: string[]
  resources: ResourceSelection[]
}

export interface PolicyDraft {
  name: string
  description: string
  statements: DraftStatement[]
}

export const ALL_ACTIONS = '*'

const APP = 'app:'
const DATABASE = 'database:'
const ENVIRONMENT = 'environment:'
const ENVIRONMENT_KIND = 'environment-kind:'

export function emptyStatement(): DraftStatement {
  return { effect: 'Allow', actions: [], resources: [] }
}

export function emptyDraft(): PolicyDraft {
  return { name: '', description: '', statements: [emptyStatement()] }
}

export function selectionToResources(sel: ResourceSelection): string[] {
  switch (sel.kind) {
    case 'all':
      return ['*']
    case 'app':
      return [APP + sel.name]
    case 'database':
      return [DATABASE + sel.name]
    case 'environment':
      return [ENVIRONMENT + sel.id]
    case 'environment_kind':
      return [ENVIRONMENT_KIND + sel.value]
    case 'project':
      return sel.environmentIds.map((id) => ENVIRONMENT + id)
    case 'pattern':
      return [sel.value]
  }
}

export function resourceToSelection(r: string): ResourceSelection {
  if (r === '*') return { kind: 'all' }
  const wildcard = r.includes('*')
  if (!wildcard && r.startsWith(ENVIRONMENT_KIND)) {
    return {
      kind: 'environment_kind',
      value: r.slice(ENVIRONMENT_KIND.length),
    }
  }
  if (!wildcard && r.startsWith(ENVIRONMENT)) {
    return { kind: 'environment', id: r.slice(ENVIRONMENT.length) }
  }
  if (!wildcard && r.startsWith(APP)) {
    return { kind: 'app', name: r.slice(APP.length) }
  }
  if (!wildcard && r.startsWith(DATABASE)) {
    return { kind: 'database', name: r.slice(DATABASE.length) }
  }
  return { kind: 'pattern', value: r }
}

export function statementResources(s: DraftStatement): string[] {
  const out: string[] = []
  for (const sel of s.resources) {
    for (const r of selectionToResources(sel)) {
      if (!out.includes(r)) out.push(r)
    }
  }
  return out
}

export function draftToDocument(draft: PolicyDraft): PolicyDocument {
  return {
    Statement: draft.statements.map((s) => ({
      Effect: s.effect,
      Action: s.actions.includes(ALL_ACTIONS) ? [ALL_ACTIONS] : [...s.actions],
      Resource: statementResources(s),
    })),
  }
}

export function documentToStatements(doc: PolicyDocument): DraftStatement[] {
  return doc.Statement.map((s) => ({
    effect: s.Effect,
    actions: [...s.Action],
    resources: s.Resource.map(resourceToSelection),
  }))
}

export function documentFromJson(
  raw: string,
): { doc: PolicyDocument } | { error: string } {
  try {
    const parsed: unknown = JSON.parse(raw)
    if (
      typeof parsed !== 'object' ||
      parsed === null ||
      !Array.isArray((parsed as { Statement?: unknown }).Statement)
    ) {
      return { error: 'Statement' }
    }
    return { doc: parsed as PolicyDocument }
  } catch (e) {
    return { error: e instanceof Error ? e.message : String(e) }
  }
}

export function principalKey(p: {
  principal_type: string
  principal_id: string
}): string {
  return `${p.principal_type}:${p.principal_id}`
}

export function selectionKey(sel: ResourceSelection): string {
  return selectionToResources(sel).join(',') + sel.kind
}

/** Issues from the server are paths like Statement[1].Action[0]; returns the ones for one statement and field. */
export function issuesFor<T extends { path: string }>(
  issues: T[],
  statement: number,
  field: 'Effect' | 'Action' | 'Resource',
): T[] {
  const prefix = `Statement[${statement}].${field}`
  return issues.filter(
    (i) => i.path === prefix || i.path.startsWith(`${prefix}[`),
  )
}
